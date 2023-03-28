package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

var Reset = "\033[0m"
var Green = "\033[32m"
var Gray = "\033[37m"

func jsonStringFromReader(reader io.Reader) string {
	var reply interface{}
	err := json.NewDecoder(reader).Decode(&reply)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to parse json response; %v\n", err)
	}
	return jsonString(reply)
}

func jsonString(reply interface{}) (str string) {
	bytes, err := json.MarshalIndent(reply, "  ", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n\nunable to marshal content; %v\n\n", err)
	} else {
		str = string(bytes)
	}
	return str
}

func trunc(str string, max int) string {
	return fmt.Sprintf("%.*s", max, str)
}

func StripBrackets(val string) string {
	if len(val) > 1 && strings.HasPrefix(val, "{") && strings.HasSuffix(val, "}") {
		val = val[1 : len(val)-1]
	}
	return val
}

func login() bool {

	// turn off certificate check
	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	client := http.Client{Transport: nil, CheckRedirect: nil, Jar: Jar}
	if Verbose {
		fmt.Fprintf(os.Stdout, "%slogging in using URL: %s\n", Reset, SitecoreLogin)
	}
	resp, err := client.PostForm(SitecoreLogin, url.Values{
		"__EVENTTARGET":        {""},
		"__VIEWSTATEGENERATOR": {"C43BEF34"},
		"UserName":             {SitecoreUser},
		"Password":             {SitecorePwd},
		"ctl07":                {"Log+in"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to post login form: %v\n", err)
	}
	resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 403 {
		fmt.Fprintf(os.Stderr, "unable to authenticate, exiting\n")
		if Verbose {
			fmt.Fprintf(os.Stderr, string(b))
		}
		os.Exit(-1)
	} else {
		cookies := Jar.Cookies(SitecoreHostURL)
		if len(cookies) > 2 && isLoggedIn(cookies) {
			if Verbose {
				fmt.Fprintf(os.Stdout, "logged in, response %d\n", resp.StatusCode)
			}
			return true
		} else {
			fmt.Fprintf(os.Stderr, "login failed, please check your credentials\n")
		}
	}
	return false
}

func isLoggedIn(cookies []*http.Cookie) bool {
	for _, v := range cookies {
		if v.Name == ".ASPXAUTH" && len(v.Value) > 8 {
			return true
		}
	}
	return false
}

func logout() {

	client := http.Client{Transport: nil, CheckRedirect: nil, Jar: Jar}
	fmt.Fprintf(os.Stdout, "\n%s", Reset)
	if Verbose {
		fmt.Fprintf(os.Stdout, "logging out using URL: %s\n", SitecoreLogout)
	}

	resp, err := client.PostForm(SitecoreLogout, url.Values{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%sunable to post logout: %v\n", Reset, err)
	}
	resp.Body.Close()

	b, _ := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == 403 {
		fmt.Fprintf(os.Stderr, "\n%sunable to logout, exiting\n", Reset)
		fmt.Fprintf(os.Stderr, string(b))
		os.Exit(-1)
	}
}
