package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"time"

	"github.com/playwright-community/playwright-go"
)

func main() {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}

	headless := true
	if os.Getenv("HEADLESS") == "false" {
		headless = false
	}

	browser, err := pw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})

	page, err := browser.NewPage()
	if err != nil {
		log.Fatalf("could not create page: %v", err)
	}

	_, err = page.Goto("https://www.americanexpress.com/ja-jp/account/login")

	if err != nil {
		log.Fatalf("could not goto: %v", err)
	}

	page.Locator("#eliloUserID").Fill(os.Getenv("AMEX_USER_ID"))
	page.Locator("#eliloPassword").Fill(os.Getenv("AMEX_PASSWORD"))

	err = page.Locator("#loginSubmit").Click()
	if err != nil {
		log.Fatal(err)
	}

	page.WaitForURL("https://global.americanexpress.com/dashboard")

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
	defer f.Close()

	csv, err := io.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(csv))

	if err := browser.Close(); err != nil {
		log.Fatalf("could not close browser: %v\n", err)
	}

	if err := pw.Stop(); err != nil {
		log.Fatalf("could not stop Playwright: %v\n", err)
	}
}
