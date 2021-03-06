package rtime

import (
	"net/http"
	"sort"
	"sync"
	"time"
)

var sites = []string{
	"facebook.com", "microsoft.com", "amazon.com", "google.com",
	"youtube.com", "twitter.com", "reddit.com", "netflix.com",
	"bing.com",
}

var rmu sync.Mutex
var rtime time.Time

// Now returns the current remote time. If the remote time cannot be
// retrieved then the zero value for Time is returned. It's a good idea to
// test for zero after every call, such as:
//
//    now := rtime.Now()
//    if now.IsZero() {
//        ... handle failure ...
//    }
//
func Now() time.Time {
	var mu sync.Mutex
	res := make([]time.Time, 0, len(sites))
	cond := sync.NewCond(&mu)
	var timedout bool

	// get as many dates as quickly as possible
	client := http.Client{
		Timeout: time.Duration(time.Second * 2),
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	for _, site := range sites {
		go func(site string) {
			resp, err := client.Head("https://" + site)
			if err == nil {
				tm, err := time.Parse(time.RFC1123, resp.Header.Get("Date"))
				resp.Body.Close()
				if err == nil {
					mu.Lock()
					res = append(res, tm)
					cond.Broadcast()
					mu.Unlock()
				}
			}
		}(site)
	}

	go func() {
		// timeout after two second
		time.Sleep(time.Second * 2)
		mu.Lock()
		timedout = true
		cond.Broadcast()
		mu.Unlock()
	}()

	mu.Lock()
	defer mu.Unlock()
	for {
		if len(res) >= 3 {
			// We must have a minimum of three results. Find the two of three
			// that have the least difference in time and take the smaller of
			// the two.
			type pair struct {
				tm0  time.Time
				tm1  time.Time
				diff time.Duration
			}
			var list []pair
			for i := 0; i < len(res); i++ {
				for j := i + 1; j < len(res); j++ {
					if i != j {
						tm0, tm1 := res[i], res[j]
						if tm0.After(tm1) {
							tm0, tm1 = tm1, tm0
						}
						list = append(list, pair{tm0, tm1, tm1.Sub(tm0)})
					}
				}
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].diff < list[j].diff {
					return true
				}
				if list[i].diff > list[j].diff {
					return false
				}
				return list[i].tm0.Before(list[j].tm0)
			})
			res := list[0].tm0.Local()
			// Ensure that the new time is after the previous time.
			rmu.Lock()
			defer rmu.Unlock()
			if res.After(rtime) {
				rtime = res
			}
			return rtime
		}
		if timedout {
			return time.Time{}
		}
		cond.Wait()
	}
}
