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
	twoskyConfFile   = "../../.twosky.json"
	localesDir       = "../../client/src/__locales"
	defaultBaseFile  = "en.json"
	defaultProjectID = "home"
	srcDir           = "../../client/src"
	twoskyURI        = "https://twosky.int.agrd.dev/api/v1"

	readLimit = 1 * 1024 * 1024
)

// langcode is a language code.
type langcode string

// languages is a map, where key is language code and value is display name.
type languages map[langcode]string

// textlabel is a text label.
type txtlabel string

// locales is a map, where key is text label and value is translation.
type locales map[txtlabel]string

func main() {
	if len(os.Args) == 1 {
		usage("need a command")
	}

	uriStr := os.Getenv("TWOSKY_URI")
	if uriStr == "" {
		uriStr = twoskyURI
	}

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
			count = 1
		}

		err = download(uriStr, projectID, conf.Languages, count)
		check(err)
	case "unused":
		err = unused()
		check(err)
	case "upload":
		err = upload(uriStr, projectID, conf.BaseLangcode)
		check(err)
	case "help", "-help", "--help":
		usage("")
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

// mark is a simple error-printing helper for scripts.
func mark(err error) {
	if err != nil {
		log.Println(err)
	}
}

// usage prints usage.  If addStr is not empty print addStr and exit with code
// 1, otherwise exit with code 0.
func usage(addStr string) {
	const usageStr = `Usage: go run main.go <command> [<args>]
Commands:
  help
    	Print usage
  summary
    	Print summary
  download [-n <count>]
		Download translations. count is a number of concurrent downloads.
  unused
    	Print unused strings
  upload
    	Upload translations`

	if addStr != "" {
		fmt.Printf("%s\n%s\n", addStr, usageStr)

		os.Exit(1)
	}

	fmt.Println(usageStr)

	os.Exit(0)
}

// readTwoskyConf returns configuration.
func readTwoskyConf() (t twoskyConf, err error) {
	b, err := os.ReadFile(twoskyConfFile)
	if err != nil {
		return twoskyConf{}, err
	}

	var conf []twoskyConf
	err = json.Unmarshal(b, &conf)
	if err != nil {
		err = errors.Annotate(err, "unmarshalling %s: %w", twoskyConfFile)

		return twoskyConf{}, err
	}

	if len(conf) == 0 {
		err = fmt.Errorf("%s is empty", twoskyConfFile)

		return twoskyConf{}, err
	}

	return conf[0], nil
}

// readLocales reads file with name fn and returns a map, where key is text
// label and value is localization.
func readLocales(fn string) (loc locales, err error) {
	b, err := os.ReadFile(fn)
	if err != nil {
		return nil, err
	}

	loc = make(locales)
	err = json.Unmarshal(b, &loc)
	if err != nil {
		err = errors.Annotate(err, "unmarshalling %s: %w", fn)

		return nil, err
	}

	return loc, nil
}

// summary prints summary for translations.
func summary(lns languages) (err error) {
	basePath := filepath.Join(localesDir, defaultBaseFile)
	baseLoc, err := readLocales(basePath)
	if err != nil {
		return err
	}

	size := float64(len(baseLoc))

	sum := make(map[langcode]float64)

	for ln := range lns {
		path := filepath.Join(localesDir, string(ln)+".json")
		if path == basePath {
			continue
		}

		var loc locales
		loc, err = readLocales(path)
		if err != nil {
			return err
		}

		sum[ln] = float64(len(loc)) * 100 / size
	}

	printSummary(sum)

	return nil
}

// printSummary to stdout.
func printSummary(sum map[langcode]float64) {
	keys := maps.Keys(sum)
	slices.Sort(keys)

	for _, v := range keys {
		fmt.Printf("%s\t %6.2f\n", v, sum[v])
	}
}

// locURL contains language code and URL of translation.
type locURL struct {
	code langcode
	url  string
}

// download translations.  w is number of workers.
func download(uriStr, projectID string, lns languages, w int) (err error) {
	downloadURI, err := url.JoinPath(uriStr, "download")
	if err != nil {
		return err
	}

	locCh := make(chan locURL)

	for i := 0; i < w; i++ {
		go downloadWorker(locCh)
	}

	for ln := range lns {
		if ln == "" {
			return errors.Error("language is empty")
		}

		v := url.Values{}
		v.Set("format", "json")
		v.Set("language", string(ln))
		v.Set("filename", defaultBaseFile)
		v.Set("project", projectID)

		uri := downloadURI + "?" + v.Encode()

		loc := locURL{ln, uri}

		locCh <- loc
	}

	close(locCh)

	return nil
}

// downloadWorker downloads translations by received urls.
func downloadWorker(locCh <-chan locURL) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for loc := range locCh {
		resp, err := client.Get(loc.url)
		if err != nil {
			mark(err)

			continue
		}

		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("url: %s; status code: %s", loc.url, http.StatusText(resp.StatusCode))
			mark(err)

			continue
		}

		limitReader, err := aghio.LimitReader(resp.Body, readLimit)
		if err != nil {
			mark(err)

			continue
		}

		buf, err := io.ReadAll(limitReader)
		if err != nil {
			mark(err)

			continue
		}

		path := filepath.Join(localesDir, string(loc.code)+".json")
		err = os.WriteFile(path, buf, 0o664)
		mark(err)

		err = resp.Body.Close()
		mark(err)

		fmt.Println(path)
	}
}

// unused prints unused text labels.
func unused() (err error) {
	fileNames := []string{}
	basePath := filepath.Join(localesDir, defaultBaseFile)
	baseLoc, err := readLocales(basePath)
	if err != nil {
		return err
	}

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if strings.HasPrefix(path, localesDir) {
			return nil
		}

		if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".json") {
			fileNames = append(fileNames, path)
		}

		return nil
	})

	if err != nil {
		return errors.Annotate(err, "filepath walking %s: %w", srcDir)
	}

	err = removeUnused(fileNames, baseLoc)
	if err != nil {
		return errors.Annotate(err, "unused: %w")
	}

	return nil
}

func removeUnused(fileNames []string, loc locales) (err error) {
	knownUsed := []txtlabel{
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

// upload base translation.
func upload(uriStr, projectID string, baseLn langcode) (err error) {
	uploadURI, err := url.JoinPath(uriStr, "upload")
	if err != nil {
		return errors.Annotate(err, "upload join path: %w")
	}

	ln := os.Getenv("UPLOAD_LANGUAGE")
	if ln == "" {
		ln = string(baseLn)
	}

	v := url.Values{}
	v.Set("format", "json")
	v.Set("language", ln)
	v.Set("filename", defaultBaseFile)
	v.Set("project", projectID)

	basePath := filepath.Join(localesDir, defaultBaseFile)
	b, err := os.ReadFile(basePath)
	if err != nil {
		return err
	}

	url := uploadURI + "?" + v.Encode()

	var buf bytes.Buffer
	buf.Write(b)

	var client http.Client
	resp, err := client.Post(url, "application/json", &buf)
	if err != nil {
		return errors.Annotate(err, "upload client post: %w")
	}

	defer func() {
		err = resp.Body.Close()
		mark(err)
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code is not ok: %s", http.StatusText(resp.StatusCode))
	}

	return nil
}

// twoskyConf is the configuration structure for localization.
type twoskyConf struct {
	Languages        languages `json:"languages"`
	ProjectID        string    `json:"project_id"`
	BaseLangcode     langcode  `json:"base_locale"`
	LocalizableFiles []string  `json:"localizable_files"`
}
