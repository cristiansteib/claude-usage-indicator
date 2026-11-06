// claude-usage-indicator fetches Claude usage (politely, with caching and
// rate-limit backoff) and publishes it for the GNOME Shell extension.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// version is set at build time: -ldflags "-X main.version=..."
var version = "dev"

func main() {
	ver := flag.Bool("version", false, "print the version and exit")
	show := flag.Bool("show", false, "print the cached state and exit (no network)")
	flag.Parse()
	if *ver {
		fmt.Println(version)
		return
	}

	svc := newService()
	if *show {
		st := svc.st
		fmt.Printf("session %.0f%% | weekly %.0f%% | fetched %s | blocked until %s | err %q\n",
			st.Usage.SessionPct, st.Usage.WeeklyPct,
			time.Unix(st.FetchedAt, 0).Format(time.DateTime),
			time.Unix(st.BlockedUntil, 0).Format(time.DateTime), st.LastError)
		d := computeDaily(time.Now(), st.Usage)
		fmt.Printf("daily: %+v\nforecast: %+v\n", d, computeForecast(time.Now(), st.Usage, st.Samples, d))
		return
	}
	if err := serveDBus(svc); err != nil {
		log.Fatal(err)
	}
	log.Println(svc.Refresh(false)) // uses the cache when it is still fresh

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	// Tick often (cheap: no network unless the policy allows it) so the
	// published file stays current and backoff expiry is noticed promptly.
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if m := svc.Refresh(false); m == "Updated" {
				log.Println(m)
			}
		case <-sig:
			return
		}
	}
}
