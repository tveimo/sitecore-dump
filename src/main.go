package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh/terminal"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	Depth           int
	Output          string
	Binaries        string
	RootID          string
	Assets          bool
	Write           bool
	WriteBinaries   bool
	Limited         int
	Written         int
	WrittenBinaries int
	Prefix          string
	Verbose         bool
	SitecoreLogin   string
	SitecoreLogout  string
	SitecoreUser    string
	SitecorePwd     string
	SitecoreHost    string
	SitecoreURL     string
	SitecoreHostURL *url.URL
	SitecoreMedia   string
	Start           time.Time
	TerminalWidth   int
	Errors          int
	AssetTemplates  map[string]int

	Jar *cookiejar.Jar
)

func configure() {
	// cookie jar to keep auth & session cookies
	jar, err := cookiejar.New(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create cookie jar for http requests; %v", err)
		os.Exit(-1)
	} else {
		Jar = jar
	}

	// signal handler for ctrl-c, so we can sign out
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logout()
		os.Exit(1)
	}()

	TerminalWidth, _, _ = terminal.GetSize(0)

	// asset templates we match to check if to store as binary, might need to be extended
	AssetTemplates = map[string]int{
		"audio": 1, "doc": 1, "document": 1, "docx": 1, "file": 1, "flash": 1,
		"image": 1, "jpeg": 1, "movie": 1, "mp3": 1, "pdf": 1, "zip": 1}
}

func parseArgs() {
	flag.BoolVar(&Write, "w", false, "store item data ")
	flag.BoolVar(&WriteBinaries, "wb", false, "store binary data")
	flag.StringVar(&Prefix, "prefix", "", "specify prefix to limit processing")
	flag.StringVar(&RootID, "root", "{11111111-1111-1111-1111-111111111111}", "specify root id to start from")
	flag.IntVar(&Depth, "d", -1, "if specified will limit traversal depth from root")
	flag.StringVar(&Output, "o", "output", "specify output folder, default: output")
	flag.StringVar(&Binaries, "b", "binaries", "specify binaries folder, default: binaries")
	flag.StringVar(&SitecoreUser, "user", "", "sitecore username")
	flag.StringVar(&SitecorePwd, "pass", "", "sitecore password")
	flag.StringVar(&SitecoreHost, "host", "", "sitecore hostname or ip address (no protocol prefix)")
	flag.BoolVar(&Verbose, "v", false, "verbose logging")

	flag.Parse()

	var err error
	SitecoreHostURL, err = url.Parse(fmt.Sprintf("https://%s", SitecoreHost))
	if err != nil {
		fmt.Fprintf(os.Stderr, "unknown sitecore host, %v\n\n", err)
		flag.PrintDefaults()
		os.Exit(-1)
	}

	SitecoreLogin = fmt.Sprintf("https://%s/sitecore/login", SitecoreHost)
	SitecoreLogout = fmt.Sprintf("https://%s/api/sitecore/Authentication/Logout?sc_database=master", SitecoreHost)
	SitecoreURL = fmt.Sprintf("https://%s/-/item/v99", SitecoreHost)
	SitecoreMedia = fmt.Sprintf("https://%s/~/media/", SitecoreHost)
}

func main() {
	Start = time.Now()

	configure()
	parseArgs()

	if login() {

		fmt.Fprintf(os.Stdout, "recursing, rootID: %s, depth: %d\n", RootID, Depth)
		_, err := fetch(RootID, true, 0, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to perform operation; %v\n", err)
			return
		}
		fmt.Fprintf(os.Stdout, "\n")

		elapsed := time.Since(Start)
		fmt.Fprintf(os.Stdout, "\n%sProcessing took %dms\n", Reset, elapsed.Milliseconds())
		Start = time.Now()

		logout()

		fmt.Fprintf(os.Stdout, "\n%swrote %d items, %d binaries\n", Reset, Written, WrittenBinaries)
		if Limited > 0 {
			fmt.Fprintf(os.Stderr, "%sdepth traversal was halted due to depth limit reached %d times\n",
				Reset, Limited)
		}
		fmt.Fprintf(os.Stdout, "%s%d errors during processing\n", Reset, Errors)
	}
}

func setup(url string) (body []byte, reader io.Reader, err error) {
	client := &http.Client{Jar: Jar}

	var resp *http.Response
	resp, err = client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to connect to sitecore; %v\n", err)
		return
	}
	defer resp.Body.Close()

	reader = ReusableReader(resp.Body)

	body, err = ioutil.ReadAll(reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to fetch data from sitecore; %v\n", err)
		//os.Exit(-1)
	}

	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "unable to fetch data, status: %s\n", resp.Status)
		//return body, reader, err
	}
	return
}

func fetch(itemID string, traverse bool, level int, force bool) (self *Item, err error) {

	// get item itself
	item, err := fetchSelf(itemID, level)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sunable to fetch item, id: %s, err: %v\n", Reset, itemID, err)
		return
	}

	path2 := item.Path
	if strings.HasPrefix(path2, "/sitecore/media library") {
		path2 = path2[23:]
	}
	if strings.HasPrefix(path2, "/sitecore/content") {
		path2 = path2[17:]
	}

	if path2 != "" && Prefix != "" && !strings.HasPrefix(path2, Prefix) {
		//fmt.Fprintf(os.Stderr, "\n%smismatch path: %s, prefix: %s", Reset, path2, Prefix)
		return
	}

	self = &item
	if Write {
		Written++
		WriteItem(item)
	}

	// only write binaries that have the template names specified
	if _, ok := AssetTemplates[strings.ToLower(item.TemplateName)]; ok {
		WrittenBinaries++
		fetchBinary(&item)
	}

	// get child items
	if (traverse && level < Depth || Depth == -1) && item.HasChildren {
		page := 0
		count := 0

		// If sitecore has items with no assigned template, you might encounter errors retrieving them.
		// Some manual mitigation might be required in those cases. -Thor

		var childrenPayload SitecoreResults

		for {
			url := fmt.Sprintf(SitecoreURL+"?sc_itemid=%s&payload=min&scope=c&page=%d&pageSize=100", itemID, page)
			body, r, err := setup(url)
			var payload SitecoreResults

			dec := json.NewDecoder(bytes.NewReader(body))
			dec.DisallowUnknownFields()
			err = dec.Decode(&payload)
			childrenPayload.StatusCode = 200
			childrenPayload.Result.TotalCount = payload.Result.TotalCount
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n%sError during Unmarshal(), id: %s, err: %v, response:\n\n%s\n",
					Reset, itemID, err, jsonStringFromReader(r))
				break
			} else {

				// create output json files
				if payload.StatusCode == 500 {
					fmt.Fprintf(os.Stderr, "\n%sid: %s, Sitecore internal error: %s, id: %s, response:\n\n%s\n",
						Reset, itemID, payload.Error.Message, jsonStringFromReader(r))
				} else {
					childrenPayload.Result.Items = append(childrenPayload.Result.Items, payload.Result.Items...)
				}
				for _, im := range payload.Result.Items {
					child, err := fetch(im.ID, traverse, level+1, force)
					if child == nil {
						if Verbose {
							fmt.Fprintf(os.Stderr, "\n%signoring child that is nil; ID: %s\n", Reset, im.ID)
						}
						continue
					}
					if err != nil {
						fmt.Fprintf(os.Stderr, "\n%sunable to fetch item, id: %s, err: %v\n", Reset, im.ID, err)
						continue
					}
					self.Children2 = append(self.Children2, child)
				}
			}
			count += payload.Result.ResultCount
			page += 1
			if count == 0 || count >= payload.Result.TotalCount {
				break
			} else {
				if Verbose {
					fmt.Fprintf(os.Stderr, "\n%sfetching paged; %d items, %d total\n",
						Reset, count, payload.Result.TotalCount)
				}
			}

		}
		if Write {
			WriteChildren(childrenPayload, item.ID)
		}

	} else if traverse && level >= Depth && item.HasChildren {
		Limited++
	}
	return
}

func fetchSelf(itemID string, level int) (item Item, err error) {
	body, r, err := setup(SitecoreURL + "?sc_itemid=" + itemID + "&payload=full")

	var payload SitecoreResults

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	err = dec.Decode(&payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during Unmarshal(), id: %s, err: %v, response:\n\n%s\n", itemID, err, jsonStringFromReader(r))
		return item, errors.New("unable to unmarshal")
	} else {
		if payload.StatusCode == 500 {
			//fmt.Fprintf(os.Stderr, "self, id: %s; Sitecore internal error: %s", itemID, payload.Error.Message)
			return item, errors.New("sitecore error; ignoring item")
		} else {
			if payload.Result.TotalCount == 1 {
				item = payload.Result.Items[0]
			} else {
				fmt.Fprintf(os.Stderr, "wrong number of items returned, id: %s; %d", itemID, payload.Result.TotalCount)
			}
		}
	}
	path := item.Path
	//if strings.HasPrefix(path, "/sitecore/content/") {
	//	path = path[17:]
	//} else if strings.HasPrefix(path, "/sitecore/media library/") {
	//	path = path[23:]
	//} else if strings.HasPrefix(path, "/sitecore/") {
	//	path = path[9:]
	//}
	fmt.Fprintf(os.Stdout, "\033[2K\r%s%s", Green, trunc(path, TerminalWidth))

	return item, nil
}

func fetchBinary(item *Item) {

	extension := "bin"
	for _, v := range item.Fields {
		if v.Name == "Extension" {
			extension = v.Value
		}
	}

	outputFilename := Binaries + "/" + StripBrackets(item.ID) + "." + extension

	if fileInfo, err := os.Stat(outputFilename); err == nil && fileInfo.Size() > 0 {
		if Verbose {
			fmt.Fprintf(os.Stdout, "not reimporting existing binary file; %s", outputFilename)
		}
	} else {
		url := SitecoreMedia + strings.Replace(StripBrackets(item.ID), "-", "", -1) + ".ashx"
		//https://hostname/~/media/{uuid}.ashx (without brackets)
		if strings.TrimSpace(extension) == "" {
			extension = "bin"
		}

		if Verbose {
			fmt.Fprintf(os.Stderr, "\n%sfetching binary for path: %s using url: %s\n", Reset, item.Path, url)
		}

		resp, err := http.Get(url)
		if err != nil {
			fmt.Errorf("unable to connect to sitecore; %v\n", err)
			return
		}
		defer resp.Body.Close()

		file, err := os.Create(outputFilename)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%sunable to create binary file; %v\n", Reset, outputFilename)
			return
		}
		defer file.Close()

		if Verbose {
			fmt.Fprintf(os.Stdout, "\n%swriting to binary file: %s\n", Reset, file.Name())
		}

		var src io.Reader
		src = &PassThru{Reader: resp.Body}
		written, err := io.Copy(file, src)

		if err != nil {
			fmt.Fprintf(os.Stderr, "\n%sunable to write to binary file: %s; %v\n", Reset, file.Name(), err)
			return
		} else {
			if Verbose {
				fmt.Fprintf(os.Stdout, "\n%swrote %d bytes", Reset, written)
			}
			_ = written
		}
	}
	return
}

type PassThru struct {
	io.Reader
	written int
}

// Provide visual feedback for large files by wrapping the reader
func (pt *PassThru) Read(p []byte) (int, error) {
	n, err := pt.Reader.Read(p)
	pt.written++
	if err == nil {
		if pt.written%100 == 50 {
			fmt.Fprintf(os.Stdout, ".")
		}
	}
	return n, err
}

func WriteChildren(payload interface{}, ID string) {
	filename := Output + "/" + StripBrackets(ID) + "-children.json"
	out, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sunable to create output file; %v\n", Reset, err)
		return
	}
	defer out.Close()

	prettyJSON, err := json.MarshalIndent(payload, "  ", "  ")

	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sUnable to marshal item: %v\n", Reset, err)
	}

	writer := bufio.NewWriter(out)

	_, err = writer.Write(prettyJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sUnable to write json to file: %v\n", Reset, err)
	}
}

func WriteItem(item Item) {
	filename := Output + "/" + StripBrackets(item.ID) + ".json"

	out, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sunable to create output file; %v\n", Reset, err)
		return
	}
	defer out.Close()

	prettyJSON, err := json.MarshalIndent(item, "  ", "  ")

	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to marshal item: %v\n", err)
	}

	writer := bufio.NewWriter(out)

	_, err = writer.Write(prettyJSON)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to write json to file: %v\n", err)
	}
}
