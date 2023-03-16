// translations downloads translations, uploads translations, prints summary
// for translations, prints unused strings.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AdguardTeam/AdGuardHome/internal/aghio"
	"github.com/AdguardTeam/golibs/errors"
	"github.com/AdguardTeam/golibs/log"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	twoskyConfFile   = "./.twosky.json"
	localesDir       = "./client/src/__locales"
	defaultBaseFile  = "en.json"
	defaultProjectID = "home"
	srcDir           = "./client/src"
	twoskyURI        = "https://twosky.int.agrd.dev/api/v1"

	readLimit = 1 * 1024 * 1024
)

// langCode is a language code.
type langCode string

// languages is a map, where key is language code and value is display name.
type languages map[langCode]string

// textlabel is a text label of localization.
type textLabel string

// locales is a map, where key is text label and value is translation.
type locales map[textLabel]string

func main() {
	if len(os.Args) == 1 {
		usage("need a command")
	}

	if os.Args[1] == "help" {
		usage("")
	}

	uriStr := os.Getenv("TWOSKY_URI")
	if uriStr == "" {
		uriStr = twoskyURI
	}

	uri, err := url.Parse(uriStr)
	check(err)

	projectID := os.Getenv("TWOSKY_PROJECT_ID")
	if projectID == "" {
		projectID = defaultProjectID
	}

	conf, err := readTwoskyConf()
	check(err)

	switch os.Args[1] {
	case "summary":
		err = summary(conf.Languages)
		check(err)
	case "download":
		var count int
		flagSet := flag.NewFlagSet("download", flag.ExitOnError)
		flagSet.IntVar(&count, "n", 1, "number of concurrent downloads")

		err = flagSet.Parse(os.Args[2:])
		check(err)

		if count < 1 {
			usage("count must be positive")
		}

		download(uri, projectID, conf.Languages, count)
	case "unused":
		err = unused()
		check(err)
	case "upload":
		err = upload(uri, projectID, conf.BaseLangcode)
		check(err)
	default:
		usage("unknown command")
	}
}

// check is a simple error-checking helper for scripts.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// usage prints usage.  If addStr is not empty print addStr and exit with code
// 1, otherwise exit with code 0.
func usage(addStr string) {
	const usageStr = `Usage: go run main.go <command> [<args>]
Commands:
  help
        Print usage.
  summary
        Print summary.
  download [-n <count>]
        Download translations. count is a number of concurrent downloads.
  unused
        Print unused strings.
  upload
        Upload translations.`

	if addStr != "" {
		fmt.Printf("%s\n%s\n", addStr, usageStr)

		os.Exit(1)
	}

	fmt.Println(usageStr)

	os.Exit(0)
}

// twoskyConf is the configuration structure for localization.
type twoskyConf struct {
	Languages        languages `json:"languages"`
	ProjectID        string    `json:"project_id"`
	BaseLangcode     langCode  `json:"base_locale"`
	LocalizableFiles []string  `json:"localizable_files"`
}

// readTwoskyConf returns configuration.
func readTwoskyConf() (t twoskyConf, err error) {
	b, err := os.ReadFile(twoskyConfFile)
	if err != nil {
		// Don't wrap the error since it's informative enough as is.
		return twoskyConf{}, err
	}

	var tsc []twoskyConf
	err = json.Unmarshal(b, &tsc)
	if err != nil {
		err = fmt.Errorf("unmarshalling %q: %w", twoskyConfFile, err)

		return twoskyConf{}, err
	}

	if len(tsc) == 0 {
		err = fmt.Errorf("%q is empty", twoskyConfFile)

		return twoskyConf{}, err
	}

	conf := tsc[0]

	for _, lang := range conf.Languages {
		if lang == "" {
			return twoskyConf{}, errors.Error("language is empty")
		}
	}

	return conf, nil
}

// readLocales reads file with name fn and returns a map, where key is text
// label and value is localization.
func readLocales(fn string) (loc locales, err error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		// Don't wrap the error since it's informative enough as is.
		return nil, err
	}

	loc = make(locales)
	err = json.Unmarshal(b, &loc)
	if err != nil {
		err = fmt.Errorf("unmarshalling %q: %w", fn, err)

		return nil, err
	}

	return loc, nil
}

// summary prints summary for translations.
func summary(langs languages) (err error) {
	basePath := filepath.Join(localesDir, defaultBaseFile)
	baseLoc, err := readLocales(basePath)
	if err != nil {
		return fmt.Errorf("summary: %w", err)
	}

	size := float64(len(baseLoc))

	sum := make(map[langCode]float64)

	for lang := range langs {
		name := filepath.Join(localesDir, string(lang)+".json")
		if name == basePath {
			continue
		}

		var loc locales
		loc, err = readLocales(name)
		if err != nil {
			return fmt.Errorf("summary read locales: %w", err)
		}

		sum[lang] = float64(len(loc)) * 100 / size
	}

	printSummary(sum)

	return nil
}

// printSummary to stdout.
func printSummary(sum map[langCode]float64) {
	keys := maps.Keys(sum)
	slices.Sort(keys)

	for _, v := range keys {
		fmt.Printf("%s\t %6.2f\n", v, sum[v])
	}
}

// download and save all translations.  uri is the base URL.  projectID is the
// name of the project.  numWorker is the number of workers for concurrent
// download.
func download(uri *url.URL, projectID string, langs languages, numWorker int) {
	downloadURI := uri.JoinPath("download")

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	uriCh := make(chan *url.URL)

	for i := 0; i < numWorker; i++ {
		go downloadWorker(client, uriCh)
	}

	for lang := range langs {
		uri = getTranslationURL(downloadURI, defaultBaseFile, projectID, lang)

		uriCh <- uri
	}

	close(uriCh)
}

// downloadWorker downloads translations by received urls and saves them.
func downloadWorker(client *http.Client, uriCh <-chan *url.URL) {
	for uri := range uriCh {
		data, err := getTranslation(client, uri.String())
		if err != nil {
			log.Error("download worker get translation: %s", err)

			continue
		}

		q := uri.Query()
		code := q.Get("language")

		name := filepath.Join(localesDir, code+".json")
		err = os.WriteFile(name, data, 0o664)
		if err != nil {
			log.Error("download worker write file: %s", err)

			continue
		}

		fmt.Println(name)
	}
}

// getTranslation returns received translation data or error.
func getTranslation(client *http.Client, url string) (data []byte, err error) {
	resp, err := client.Get(url)
	if err != nil {
		err = fmt.Errorf("get: %w", err)

		return nil, err
	}

	defer log.OnCloserError(resp.Body, log.ERROR)

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("url: %q; status code: %s", url, http.StatusText(resp.StatusCode))

		return nil, err
	}

	limitReader, err := aghio.LimitReader(resp.Body, readLimit)
	if err != nil {
		err = fmt.Errorf("limit reader: %w", err)

		return nil, err
	}

	data, err = io.ReadAll(limitReader)
	if err != nil {
		err = fmt.Errorf("read all: %w", err)

		return nil, err
	}

	return data, nil
}

// getTranslationURL returns a new [url.URL] with provided query parameters.
func getTranslationURL(oldURL *url.URL, baseFile, projectID string, lang langCode) (uri *url.URL) {
	uri, err := url.Parse(oldURL.String())
	check(err)

	q := uri.Query()
	q.Set("format", "json")
	q.Set("filename", baseFile)
	q.Set("project", projectID)
	q.Set("language", string(lang))

	uri.RawQuery = q.Encode()

	return uri
}

// unused prints unused text labels.
func unused() (err error) {
	fileNames := []string{}
	basePath := filepath.Join(localesDir, defaultBaseFile)
	baseLoc, err := readLocales(basePath)
	if err != nil {
		return fmt.Errorf("unused: %w", err)
	}

	// We don't need no file info.  There is no place for errors.
	err = filepath.Walk(srcDir, func(name string, _ os.FileInfo, _ error) error {
		locDir := filepath.Clean(localesDir)

		if strings.HasPrefix(name, locDir) {
			return nil
		}

		if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".json") {
			fileNames = append(fileNames, name)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("filepath walking %q: %w", srcDir, err)
	}

	err = removeUnused(fileNames, baseLoc)

	return errors.Annotate(err, "remove unused: %w")
}

func removeUnused(fileNames []string, loc locales) (err error) {
	knownUsed := []textLabel{
		"blocking_mode_refused",
		"blocking_mode_nxdomain",
		"blocking_mode_custom_ip",
	}

	for _, v := range knownUsed {
		delete(loc, v)
	}

	for _, fn := range fileNames {
		var buf []byte
		buf, err = os.ReadFile(fn)
		if err != nil {
			// Don't wrap the error since it's informative enough as is.
			return err
		}

		for k := range loc {
			if !bytes.Contains(buf, []byte(k)) {
				continue
			}

			delete(loc, k)
		}
	}

	printUnused(loc)

	return nil
}

// printUnused text labels to stdout.
func printUnused(loc locales) {
	keys := maps.Keys(loc)
	slices.Sort(keys)

	for _, v := range keys {
		fmt.Println(v)
	}
}

// upload base translation.  uri is the base URL.  projectID is the name of the
// project.  baseLn is the base language code.
func upload(uri *url.URL, projectID string, baseLn langCode) (err error) {
	uploadURI := uri.JoinPath("upload")

	lang := baseLn

	langStr := os.Getenv("UPLOAD_LANGUAGE")
	if langStr != "" {
		lang = langCode(langStr)
	}

	basePath := filepath.Join(localesDir, defaultBaseFile)
	b, err := os.ReadFile(basePath)
	if err != nil {
		// Don't wrap the error since it's informative enough as is.
		return err
	}

	var buf bytes.Buffer
	buf.Write(b)

	uri = getTranslationURL(uploadURI, defaultBaseFile, projectID, lang)

	var client http.Client
	resp, err := client.Post(uri.String(), "application/json", &buf)
	if err != nil {
		return fmt.Errorf("upload: client post: %w", err)
	}

	defer func() {
		err = errors.WithDeferred(err, resp.Body.Close())
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code is not ok: %q", http.StatusText(resp.StatusCode))
	}

	return nil
}
