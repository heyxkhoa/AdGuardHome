// translations downloads translations, uploads translations, prints summary
// for translations, prints unused strings.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AdguardTeam/golibs/log"
	"golang.org/x/exp/slices"
)

const (
	TwoskyConf = "../../.twosky.json"
	LocalesDir = "../../client/src/__locales"
	BaseFile   = "en.json"
	ProjectID  = "home"
	SrcDir     = "../../client/src"

	TwoskyURI         = "https://twosky.int.agrd.dev/api/v1"
	TwoskyDownloadURI = "https://twosky.int.agrd.dev/api/v1/download"
	TwoskyUploadURI   = "https://twosky.int.agrd.dev/api/v1/upload"
)

type locale map[string]string

type languages map[string]string

func main() {
	if len(os.Args) == 1 {
		// TODO!!: usage
		fmt.Println("usage")
		return
	}

	t := readTwosky()

	switch os.Args[1] {
	case "count":
		count(t.Languages)
	case "download":
		w := 1
		if len(os.Args) > 2 {
			i, err := strconv.Atoi(os.Args[2])
			if err != nil {
				log.Fatal(err)
			}

			if i > 1 {
				w = i
			}
		}

		download(t.Languages, w)
	case "unused":
		unused()
	case "upload":
		upload()
	default:
		fmt.Println("usage")
	}
}

func readTwosky() twosky {
	b, err := os.ReadFile(TwoskyConf)
	if err != nil {
		log.Fatal(err)
	}

	var ts []twosky
	err = json.Unmarshal(b, &ts)
	if err != nil {
		log.Fatal(err)
	}

	if len(ts) == 0 {
		log.Fatal()
	}

	return ts[0]
}

func readLocale(n string) locale {
	b, err := os.ReadFile(n)
	if err != nil {
		log.Fatal(err)
	}

	var l locale
	err = json.Unmarshal(b, &l)
	if err != nil {
		log.Fatal(err)
	}

	return l
}

// count prints summary for translations.
func count(lns languages) {
	basePath := filepath.Join(LocalesDir, BaseFile)
	baseLocale := readLocale(basePath)

	sum := make(map[string]int)

	for ln := range lns {
		path := filepath.Join(LocalesDir, ln+".json")
		l := readLocale(path)

		if path == basePath {
			continue
		}

		sum[ln] = len(l) * 100 / len(baseLocale)
	}

	printSummary(sum)
}

func printSummary(sum map[string]int) {
	keys := make([]string, 0, len(sum))
	for k := range sum {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	for _, v := range keys {
		fmt.Printf("%s\t %d\n", v, sum[v])
	}
}

// download translations. w is number of workers.
func download(lns languages, w int) {
	urls := make(chan string)

	for i := 0; i < w; i++ {
		go downloadWorker(urls)
	}

	for ln := range lns {
		v := url.Values{}
		v.Set("format", "json")
		v.Set("language", ln)
		v.Set("filename", BaseFile)
		v.Set("project", ProjectID)

		urls <- TwoskyDownloadURI + "?" + v.Encode()
	}

	close(urls)
}

func downloadWorker(urls <-chan string) {
	var client http.Client

	for u := range urls {
		resp, err := client.Get(u)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(u)

		if resp.StatusCode != http.StatusOK {
			log.Fatal()
		}

		url, err := url.Parse(u)
		if err != nil {
			log.Fatal(err)
		}

		v := url.Query()
		locale := v.Get("language")

		buf, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		path := filepath.Join(LocalesDir, locale+".json")
		err = os.WriteFile(path, buf, 0o664)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(path)

		err = resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
}

// unused prints unused strings.
func unused() {
	names := []string{}
	err := filepath.Walk(SrcDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasPrefix(path, LocalesDir) {
			return nil
		}

		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".json") {
			names = append(names, path)
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	files := []string{}
	for _, n := range names {
		var buf []byte
		buf, err = os.ReadFile(n)
		if err != nil {
			log.Fatal(err)
		}

		files = append(files, string(buf))
	}

	basePath := filepath.Join(LocalesDir, BaseFile)
	baseLocale := readLocale(basePath)

	knownUsed := map[string]bool{
		"blocking_mode_refused":   true,
		"blocking_mode_nxdomain":  true,
		"blocking_mode_custom_ip": true,
	}

	unused := []string{}
	for k := range baseLocale {
		if knownUsed[k] {
			continue
		}

		used := false
		for _, f := range files {
			if strings.Contains(f, k) {
				used = true
				break
			}
		}
		if !used {
			unused = append(unused, k)
		}
	}

	for _, v := range unused {
		fmt.Println(v)
	}
}

// upload base translation.
func upload() {
	v := url.Values{}
	v.Set("format", "json")
	v.Set("language", "en")
	v.Set("filename", BaseFile)
	v.Set("project", ProjectID)

	basePath := filepath.Join(LocalesDir, BaseFile)
	b, err := os.ReadFile(basePath)
	if err != nil {
		log.Fatal(err)
	}

	url := TwoskyUploadURI + "?" + v.Encode()

	var buf bytes.Buffer
	buf.Write(b)

	var client http.Client
	resp, err := client.Post(url, "application/json", &buf)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		log.Fatal(err)
	}
}

type twosky struct {
	Languages        languages `json:"languages"`
	ProjectID        string    `json:"project_id"`
	BaseLocale       string    `json:"base_locale"`
	LocalizableFiles []string  `json:"localizable_files"`
}
