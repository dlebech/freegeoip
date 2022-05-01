// Copyright 2009 The freegeoip authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apiserver

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-web/httplog"
	"github.com/go-web/httpmux"
	"golang.org/x/text/language"

	"github.com/fiorix/freegeoip"
	"github.com/julienschmidt/httprouter"
)

type apiHandler struct {
	db   *freegeoip.DB
	conf *Config
}

// NewHandler creates an http handler for the freegeoip server that
// can be embedded in other servers.
func NewHandler(c *Config) (http.Handler, error) {
	db, err := openDB(c)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}
	f := &apiHandler{db: db, conf: c}
	chain := f.getChain()
	router := httprouter.New()
	router.HandlerFunc(http.MethodGet, "/json/:host", buildChain(f.iplookup(jsonWriter), chain...))
	go watchEvents(db)
	return router, nil
}

// buildChain builds the middlware chain recursively, functions are first class
func buildChain(f http.HandlerFunc, m ...httpmux.MiddlewareFunc) http.HandlerFunc {
	// if our chain is done, use the original handlerfunc
	if len(m) == 0 {
		return f
	}
	// otherwise nest the handlerfuncs
	return m[0](buildChain(f, m[1:cap(m)]...))
}

func (f *apiHandler) getChain() []httpmux.MiddlewareFunc {
	var chain = []httpmux.MiddlewareFunc{}
	if f.conf.UseXForwardedFor {
		chain = append(chain, httplog.UseXForwardedFor)
	}
	if !f.conf.Silent {
		chain = append(chain, httplog.ApacheCombinedFormat(f.conf.accessLogger()))
	}
	return chain
}

type writerFunc func(w http.ResponseWriter, r *http.Request, d *responseRecord)

func (f *apiHandler) iplookup(writer writerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := httprouter.ParamsFromContext(r.Context()).ByName("host")
		if len(host) > 0 && host[0] == '/' {
			host = host[1:]
		}
		if host == "" {
			host, _, _ = net.SplitHostPort(r.RemoteAddr)
			if host == "" {
				host = r.RemoteAddr
			}
		}
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			http.NotFound(w, r)
			return
		}
		ip, q := ips[rand.Intn(len(ips))], &geoipQuery{}
		err = f.db.Lookup(ip, &q.DefaultQuery)
		if err != nil {
			http.Error(w, "Try again later.", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("X-Database-Date", f.db.Date().Format(http.TimeFormat))
		resp := q.Record(ip, r.Header.Get("Accept-Language"))
		writer(w, r, resp)
	}
}

func jsonWriter(w http.ResponseWriter, r *http.Request, d *responseRecord) {
	if cb := r.FormValue("callback"); cb != "" {
		w.Header().Set("Content-Type", "application/javascript")
		io.WriteString(w, cb)
		w.Write([]byte("("))
		b, err := json.Marshal(d)
		if err == nil {
			w.Write(b)
		}
		io.WriteString(w, ");")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(d)
}

type geoipQuery struct {
	freegeoip.DefaultQuery
}

func (q *geoipQuery) Record(ip net.IP, lang string) *responseRecord {
	lang = parseAcceptLanguage(lang, q.Country.Names)

	r := &responseRecord{
		IP:          ip.String(),
		CountryCode: q.Country.ISOCode,
		CountryName: q.Country.Names[lang],
		City:        q.City.Names[lang],
		ZipCode:     q.Postal.Code,
		TimeZone:    q.Location.TimeZone,
		Latitude:    roundFloat(q.Location.Latitude, .5, 4),
		Longitude:   roundFloat(q.Location.Longitude, .5, 4),
		MetroCode:   q.Location.MetroCode,
		Continent:   q.Continent.Names[lang],
	}
	if len(q.Region) > 0 {
		r.RegionCode = q.Region[0].ISOCode
		r.RegionName = q.Region[0].Names[lang]
	}
	if val, ok := q.Continent.Names[lang]; ok {
		r.Continent = val
	}
	return r
}

func parseAcceptLanguage(header string, dbLangs map[string]string) string {
	// supported languages -- i.e. languages available in the DB
	matchLangs := []language.Tag{
		language.English,
	}

	// parse available DB languages and add to supported
	for name := range dbLangs {
		matchLangs = append(matchLangs, language.Raw.Make(name))
	}

	var matcher = language.NewMatcher(matchLangs)

	// parse header
	t, _, _ := language.ParseAcceptLanguage(header)
	// match most acceptable language
	tag, _, _ := matcher.Match(t...)
	// extract base language
	base, _ := tag.Base()

	return base.String()
}

func roundFloat(val float64, roundOn float64, places int) (newVal float64) {
	var round float64
	pow := math.Pow(10, float64(places))
	digit := pow * val
	_, div := math.Modf(digit)
	if div >= roundOn {
		round = math.Ceil(digit)
	} else {
		round = math.Floor(digit)
	}
	return round / pow
}

type responseRecord struct {
	IP          string  `json:"ip"`
	CountryCode string  `json:"country_code"`
	CountryName string  `json:"country_name"`
	RegionCode  string  `json:"region_code"`
	RegionName  string  `json:"region_name"`
	City        string  `json:"city"`
	ZipCode     string  `json:"zip_code"`
	TimeZone    string  `json:"time_zone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	MetroCode   uint    `json:"metro_code"`
	Continent   string  `json:"continent"`
}

func (rr *responseRecord) String() string {
	b := &bytes.Buffer{}
	w := csv.NewWriter(b)
	w.UseCRLF = true
	w.Write([]string{
		rr.IP,
		rr.CountryCode,
		rr.CountryName,
		rr.RegionCode,
		rr.RegionName,
		rr.City,
		rr.ZipCode,
		rr.TimeZone,
		strconv.FormatFloat(rr.Latitude, 'f', 4, 64),
		strconv.FormatFloat(rr.Longitude, 'f', 4, 64),
		strconv.Itoa(int(rr.MetroCode)),
	})
	w.Flush()
	return b.String()
}

// openDB opens and returns the IP database file or URL.
func openDB(c *Config) (*freegeoip.DB, error) {
	u, err := url.Parse(c.DB)
	if err != nil || len(u.Scheme) == 0 {
		return freegeoip.Open(c.DB)
	}
	return freegeoip.OpenURL(c.DB)
}

// watchEvents logs and collect metrics of database events.
func watchEvents(db *freegeoip.DB) {
	for {
		select {
		case file := <-db.NotifyOpen():
			log.Println("database loaded:", file)
		case err := <-db.NotifyError():
			log.Println("database error:", err)
		case msg := <-db.NotifyInfo():
			log.Println("database info:", msg)
		case <-db.NotifyClose():
			return
		}
	}
}
