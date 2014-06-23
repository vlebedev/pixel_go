package main

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/nu7hatch/gouuid"
	l4g "github.com/sberlabs/log4go"
	"github.com/sberlabs/statsd"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type JSONData struct {
	Timestamp  int64
	Cookie     string
	Url        string
	Pid        string
	RemoteAddr string
	Headers    http.Header
}

type XmlPixel struct {
	Base64      string `xml:"base64"`
	Cookie      string `xml:"cookie"`
	ChanBufSize int    `xml:"chanbufsize"`
	NodeName    string `xml:"nodename"`
	Port        string `xml:"port"`
	Path        string `xml:"path"`
	StatsD      string `xml:"statsd"`
}

type XmlConfig struct {
	Pixel XmlPixel `xml:"pixel"`
}

var (
	Config       *XmlConfig
	StatsdClient *statsd.StatsdClient
	PixelBinary  []byte
	LogChan      chan []byte

	// Flags
	ConfigFile = flag.String("config", "pixel.xml", "Pixel server config file")
)

func handler(w http.ResponseWriter, r *http.Request) {
	timeStart := int64(time.Now().UnixNano())
	cookie := get_cookie(r)
	url := r.URL.String()
	pid := r.FormValue("pid")
	remote := r.RemoteAddr
	headers := r.Header
	h := w.Header()
	h.Add("Content-Type", "image/png")
	h.Add("Connection", "close")
	http.SetCookie(w, cookie)
	w.Write(PixelBinary)
	go func() {
		data, err := json.Marshal(JSONData{
			Timestamp:  timeStart / 1000,
			Cookie:     cookie.Value,
			Url:        url,
			Pid:        pid,
			RemoteAddr: remote,
			Headers:    headers,
		})
		if err != nil {
			l4g.Warn("can't marshal data to JSON: %s", err)
		}
		LogChan <- data
	}()
	timeFinish := int64(time.Now().UnixNano())
	StatsdClient.Timing("request.time", (timeFinish-timeStart)/1000)
}

func get_cookie(r *http.Request) (result *http.Cookie) {
	cookie, err := r.Cookie(Config.Pixel.Cookie)
	if err != nil {
		u, _ := uuid.NewV4()
		result = &http.Cookie{
			Name:    Config.Pixel.Cookie,
			Value:   u.String(),
			Expires: time.Date(2020, 01, 01, 1, 0, 0, 0, time.UTC),
		}
	} else {
		result = cookie
	}
	return result
}

func logger() {
	for {
		data := <-LogChan
		l4g.Finest("%s", string(data[:]))
	}
}

func LoadConfiguration(filename string) *XmlConfig {
	// Open the configuration file
	fd, err := os.Open(filename)
	defer fd.Close()

	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfiguration: Error: Could not open %q for reading: %s\n", filename, err)
		os.Exit(1)
	}

	contents, err := ioutil.ReadAll(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfiguration: Error: Could not read %q: %s\n", filename, err)
		os.Exit(1)
	}

	config := new(XmlConfig)
	if err := xml.Unmarshal(contents, config); err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfiguration: Error: Could not parse XML configuration in %q: %s\n", filename, err)
		os.Exit(1)
	}
	return config
}

func main() {
	flag.Parse()

	// Load Configuration
	Config = LoadConfiguration(*ConfigFile)
	PixelBinary, _ = base64.StdEncoding.DecodeString(Config.Pixel.Base64)
	LogChan = make(chan ([]byte), Config.Pixel.ChanBufSize)

	// Init logger
	l4g.LoadConfiguration(*ConfigFile)
	defer l4g.Close()

	// Init profiler
	StatsdClient = statsd.NewStatsdClient(Config.Pixel.StatsD, Config.Pixel.NodeName+".")
	StatsdClient.CreateSocket()
	defer StatsdClient.Close()

	// Launch logger
	go logger()

	// Launch http server
	http.HandleFunc(Config.Pixel.Path, handler)
	http.ListenAndServe(":"+Config.Pixel.Port, nil)
}
