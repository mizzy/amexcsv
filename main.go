package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"os"

	"github.com/playwright-community/playwright-go"
)

func main() {
	pw, err := playwright.Run()
	if err != nil {
		log.Fatalf("could not start playwright: %v", err)
	}

	browser, err := pw.Firefox.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
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

	href, err := page.GetByText("ご利用履歴を確認する").GetAttribute("href")
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

	csvURL := fmt.Sprintf("https://global.americanexpress.com/api/servicing/v1/financials/documents?file_format=csv&limit=3&status=posted&account_key=%s&client_id=AmexAPI", accountKey)

	download, err := page.ExpectDownload(func() error {
		_, err := page.Goto(csvURL)
		return err
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
