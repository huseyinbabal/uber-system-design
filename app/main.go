// location-service: drivers stream their position over a WebSocket,
// riders get the drivers near them pushed once a second.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
)

//go:embed map.html
var mapPage []byte

type Location struct {
	Driver string  `json:"driver"`
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Pod    string  `json:"pod"`
	At     int64   `json:"at"`
}

var (
	pod = os.Getenv("POD_NAME")
	mu  sync.RWMutex
	// every driver this pod knows about
	drivers = map[string]Location{}
	rdb     *redis.Client
)

func save(l Location) {
	mu.Lock()
	drivers[l.Driver] = l
	mu.Unlock()
}

// driverHandler: one connection per driver, one message every few seconds
func driverHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	id := r.URL.Query().Get("id")
	ctx := r.Context()
	for {
		_, msg, err := c.Read(ctx)
		if err != nil {
			return
		}
		var l Location
		if json.Unmarshal(msg, &l) != nil {
			continue
		}
		l.Driver, l.Pod, l.At = id, pod, time.Now().UnixMilli()
		if rdb != nil {
			// every pod hears it, this one included
			b, _ := json.Marshal(l)
			rdb.Publish(ctx, "locations", b)
		} else {
			save(l)
		}
	}
}

// riderHandler: pushes the drivers within `km` of the rider every second
func riderHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.CloseNow()
	q := r.URL.Query()
	lat, _ := strconv.ParseFloat(q.Get("lat"), 64)
	lng, _ := strconv.ParseFloat(q.Get("lng"), 64)
	km, _ := strconv.ParseFloat(q.Get("km"), 64)
	ctx := c.CloseRead(r.Context())
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		b, _ := json.Marshal(map[string]any{"pod": pod, "drivers": nearby(lat, lng, km)})
		if c.Write(ctx, websocket.MessageText, b) != nil {
			return
		}
	}
}

func nearby(lat, lng, km float64) []Location {
	cutoff := time.Now().Add(-15 * time.Second).UnixMilli()
	out := []Location{}
	mu.RLock()
	defer mu.RUnlock()
	for _, d := range drivers {
		if d.At > cutoff && distanceKm(lat, lng, d.Lat, d.Lng) <= km {
			out = append(out, d)
		}
	}
	return out
}

func distanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	const r = 6371
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

func subscribe() {
	for msg := range rdb.Subscribe(context.Background(), "locations").Channel() {
		var l Location
		if json.Unmarshal([]byte(msg.Payload), &l) == nil {
			save(l)
		}
	}
}

func main() {
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		rdb = redis.NewClient(&redis.Options{Addr: addr})
		go subscribe()
	}
	http.HandleFunc("/driver", driverHandler)
	http.HandleFunc("/rider", riderHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.Write(mapPage) })
	log.Printf("pod %s listening on :8080 (redis: %v)", pod, rdb != nil)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
