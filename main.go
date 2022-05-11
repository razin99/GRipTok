package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

const URL = "https://tiktok.com/"

func MustProtoToJar(protoCookies []*proto.NetworkCookie) *cookiejar.Jar {
	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Fatalln(err)
	}
	for _, cookie := range protoCookies {
		u, err := url.Parse(cookie.Domain)
		if err != nil {
			log.Fatalln(err)
		}
		jar.SetCookies(u, []*http.Cookie{
			{Name: cookie.Name, Value: cookie.Value},
		})
	}
	return jar
}

func GetLastSlugs(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		log.Println(err)
	}
	slugs := strings.Split(u.Path, "/")
	return slugs[len(slugs)-1]
}

func SaveToFile(filepath string, src io.Reader) error {
	out, err := os.Create(filepath)
	if err != nil {
		log.Fatalln(err)
	}
	defer out.Close()
	_, err = io.Copy(out, src)
	return err
}

func WaitOnElem(el *rod.Element) *rod.Element {
	return el.
		MustWaitLoad().
		MustWaitStable().
		MustWaitVisible().
		MustWaitEnabled()
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(
			flag.CommandLine.Output(),
			"%s [...options] username\n", os.Args[0],
		)
		flag.PrintDefaults()
	}
	var outDir string
	flag.StringVar(&outDir, "output-dir", "output", "Output directory")
	var maxThreads int64
	flag.Int64Var(&maxThreads, "threads", int64(runtime.NumCPU()), "Max parallel downloads")
	flag.Parse()

	username := flag.Arg(0)
	if username == "" {
		log.Fatalln("Username must not be empty")
	}
	username = "@" + username
	urlPattern := regexp.MustCompile(".*/" + regexp.QuoteMeta(username+"/video") + "/.*")

	if err := os.Mkdir(outDir, os.ModePerm); err != nil {
		log.Println(err)
	}
	if err := os.Mkdir(path.Join(outDir, username[1:]), os.ModePerm); err != nil {
		log.Println(err)
	}

	browser := rod.New().ControlURL(
		launcher.New().MustLaunch(),
	).MustConnect()
	defer browser.MustClose()

	page := stealth.MustPage(browser)
	page.MustNavigate(URL + username).MustWaitLoad()

	var firstVid *rod.Element
	for _, a := range page.MustElements("a") {
		link := a.MustProperty("href").Str()
		if !urlPattern.MatchString(link) {
			continue
		} else {
			firstVid = a
			break
		}
	}
	if firstVid == nil {
		log.Fatalln("Cannot find first video URL")
	}
	firstVid.MustClick().MustWaitLoad()

	jar := MustProtoToJar(page.MustCookies())
	client := &http.Client{Jar: jar}
	sem := semaphore.NewWeighted(maxThreads)
	ctx := context.Background()
	ctr := 0
	timeoutDuration := time.Second * 3

	currPage := page.MustInfo().URL
	prevPage := ""
	for currPage != prevPage {
		el, err := page.
			MustWaitIdle().
			MustWaitLoad().
			Timeout(timeoutDuration).
			Element("video")
		WaitOnElem(el)
		if err != nil {
			log.Fatalln("No video found, URL: ", currPage)
		}
		if err := sem.Acquire(ctx, 1); err != nil {
			log.Fatalln(err)
		}
		link := string(*el.Timeout(timeoutDuration).MustAttribute("src"))
		go func(link string, currPage string) {
			defer sem.Release(1)
			res, err := client.Get(link)
			if err != nil {
				log.Fatalln(err)
			}
			defer res.Body.Close()
			id := GetLastSlugs(currPage)
			savePath := path.Join(outDir, username[1:], fmt.Sprintf("%s.mp4", id))
			if err := SaveToFile(savePath, res.Body); err != nil {
				log.Fatalln(err)
			}
			log.Println("Saved video ", savePath)
		}(link, currPage)
		ctr++

		page.Keyboard.MustPress(input.ArrowDown)
		page.
			Timeout(timeoutDuration).
			MustWaitIdle().
			MustWaitLoad().
			MustWaitElementsMoreThan("video", 0)
		WaitOnElem(page.MustElement("video"))
		prevPage = currPage
		currPage = page.MustInfo().URL
	}
	if err := sem.Acquire(ctx, maxThreads); err != nil {
		log.Fatalln(err)
	}
	sem.Release(maxThreads)
	log.Println("Videos iterated over: ", ctr)
}
