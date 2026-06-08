package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/playwright-community/playwright-go"
)

const (
	chromeBinary = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	debugPort    = "9222"
	cdpEndpoint  = "http://127.0.0.1:9222"
)

func main() {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}

	chromeCmd, err := ensureChromeRunning()
	if err != nil {
		log.Fatalf("could not start chrome: %v", err)
	}

	wsURL, err := fetchWebSocketDebuggerURL()
	if err != nil {
		log.Fatalf("could not fetch CDP endpoint: %v", err)
	}

	browser, err := pw.Chromium.ConnectOverCDP(wsURL)
	if err != nil {
		log.Fatalf("could not connect over CDP: %v", err)
	}

	contexts := browser.Contexts()
	if len(contexts) == 0 {
		log.Fatalf("no browser contexts found")
	}
	ctx := contexts[0]

	page, err := ctx.NewPage()
	if err != nil {
		log.Fatalf("could not create page: %v", err)
	}

	if _, err := page.Goto("https://global.americanexpress.com/dashboard"); err != nil {
		log.Fatalf("could not goto dashboard: %v", err)
	}

	if !isDashboard(page.URL()) {
		log.Println("not logged in, performing login")
		if err := login(page); err != nil {
			log.Fatalf("login failed: %v", err)
		}
	}

	href, err := page.GetByTitle("ご利用履歴").Nth(0).GetAttribute("href")
	if err != nil {
		log.Fatal(err)
	}

	u, err := url.Parse(href)
	if err != nil {
		log.Fatal(err)
	}

	var accountKey string
	for key, values := range u.Query() {
		if key == "account_key" {
			accountKey = values[0]
		}
	}

	csvURL, _ := url.Parse("https://global.americanexpress.com/api/servicing/v1/financials/documents")

	q := csvURL.Query()
	q.Set("file_format", "csv")
	q.Set("limit", "200")
	q.Set("status", "posted")
	q.Set("account_key", accountKey)

	// 締め日の翌日は、締め日までのCSVを取得
	now := time.Now()
	if now.Day() == 6 {
		date := now.Add(-24 * time.Hour)
		q.Set("statement_end_date", fmt.Sprintf("%s", date.Format("2006-01-02")))
	}

	csvURL.RawQuery = q.Encode()

	download, err := page.ExpectDownload(func() error {
		page.Goto(csvURL.String())
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	csvFile, err := download.Path()
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Open(csvFile)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	csv, err := io.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(csv))

	page.Close()

	if chromeCmd != nil {
		if err := chromeCmd.Process.Signal(os.Interrupt); err != nil {
			_ = chromeCmd.Process.Kill()
		}
		_, _ = chromeCmd.Process.Wait()
	}

	if err := pw.Stop(); err != nil {
		log.Fatalf("could not stop Playwright: %v\n", err)
	}
}

func ensureChromeRunning() (*exec.Cmd, error) {
	if _, err := http.Get(cdpEndpoint + "/json/version"); err == nil {
		return nil, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	userDataDir := filepath.Join(home, ".cache", "amexcsv", "chrome-profile")
	if err := os.MkdirAll(userDataDir, 0o700); err != nil {
		return nil, err
	}

	cmd := exec.Command(chromeBinary,
		"--remote-debugging-port="+debugPort,
		"--user-data-dir="+userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
	)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := http.Get(cdpEndpoint + "/json/version"); err == nil {
			return cmd, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	return nil, fmt.Errorf("chrome did not become reachable on %s", cdpEndpoint)
}

func fetchWebSocketDebuggerURL() (string, error) {
	resp, err := http.Get(cdpEndpoint + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("empty webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}

func isDashboard(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	return parsed.Host == "global.americanexpress.com" && parsed.Path == "/dashboard"
}

func login(page playwright.Page) error {
	if _, err := page.Goto("https://www.americanexpress.com/ja-jp/account/login"); err != nil {
		return fmt.Errorf("goto login: %w", err)
	}
	if _, err := page.Locator("select[data-testid=menu-dropdown]").SelectOption(playwright.SelectOptionValues{
		Values: &[]string{"account"},
	}); err != nil {
		return fmt.Errorf("select account type: %w", err)
	}
	if err := page.Locator("#eliloUserID").Fill(os.Getenv("AMEX_USER_ID")); err != nil {
		return fmt.Errorf("fill user id: %w", err)
	}
	if err := page.Locator("#eliloPassword").Fill(os.Getenv("AMEX_PASSWORD")); err != nil {
		return fmt.Errorf("fill password: %w", err)
	}
	if err := page.Locator("#loginSubmit").Click(); err != nil {
		return fmt.Errorf("click login: %w", err)
	}
	if err := page.WaitForURL("https://global.americanexpress.com/dashboard"); err != nil {
		return fmt.Errorf("wait for dashboard: %w", err)
	}
	return nil
}
