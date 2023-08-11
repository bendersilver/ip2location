package ip2location

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/gob"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bendersilver/jlog"
)

const dbURL = "https://download.ip2location.com/lite/IP2LOCATION-LITE-DB1.CSV.ZIP"
const gobFile = "/tmp/ip2location.lite.bob"

var mx sync.Mutex

type ip2location struct {
	IP4        []uint32
	Country    []string
	NextUpdate time.Time
}

func randInt(max int) int {
	return rand.Intn(max)
}

func (i *ip2location) timer() {
	f, _ := os.Open(gobFile)
	if f != nil {
		gob.NewDecoder(f).Decode(&i)
	}
	var err error
	for range time.Tick(time.Second) {
		if time.Now().After(i.NextUpdate) {
			err = i.update()
			if err != nil {
				jlog.Error(err)
			}
		}
	}
}

func (i *ip2location) update() error {
	res, err := http.Get(dbURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	rd := bytes.NewReader(b)
	zrd, err := zip.NewReader(rd, int64(rd.Len()))
	if err != nil {
		return err
	}

	var startIP uint64
	var c string
	for _, file := range zrd.File {
		if file.Name == "IP2LOCATION-LITE-DB1.CSV" {
			frd, err := file.Open()
			if err != nil {
				return err
			}
			defer frd.Close()

			rcsv := csv.NewReader(frd)
			mx.Lock()
			defer mx.Unlock()
			for {
				record, err := rcsv.Read()
				if err == io.EOF {
					break
				}

				if err != nil {
					return err
				}

				startIP, err = strconv.ParseUint(record[0], 10, 32)
				if err != nil {
					jlog.Error(err)
					continue
				}
				i.IP4 = append(i.IP4, uint32(startIP))
				c = record[2]
				if c == "-" {
					c = "--"
				}
				i.Country = append(i.Country, c)
			}
		}
	}

	f, err := os.OpenFile(gobFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	t := time.Now()
	i.NextUpdate = time.Date(t.Year(), t.Month()+1, randInt(7), randInt(23), randInt(59), randInt(59), 0, t.Location())
	jlog.Notice("ip2location update success")
	return gob.NewEncoder(f).Encode(&i)
}

func (i *ip2location) countryCode(ip string) string {
	mx.Lock()
	defer mx.Unlock()
	n := binary.BigEndian.Uint32(net.ParseIP(ip).To4())
	ix, j := 0, len(i.IP4)
	for ix < j {
		h := (ix + j) >> 1
		if i.IP4[h] > n {
			j = h
		} else {
			ix = h + 1
		}
	}
	if ix < 1 {
		return "--"
	}
	return i.Country[ix-1]
}

func Country(ip string) string {
	return loc.countryCode(ip)
}

var loc *ip2location

func init() {
	loc = new(ip2location)
	go loc.timer()
}
