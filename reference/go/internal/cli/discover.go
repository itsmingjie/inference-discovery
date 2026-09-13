package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

func discover(ctx context.Context, scan func() ([]provider, error), o options, out io.Writer) error {
	previous := map[string]string{}
	for {
		ps, err := scan()
		if err != nil {
			return err
		}
		if !o.watch {
			if o.json {
				return json.NewEncoder(out).Encode(ps)
			}
			if len(ps) == 0 {
				fmt.Fprintln(out, "No providers found. Try --descriptor-url when multicast is unavailable.")
			}
			for _, p := range ps {
				display(out, p, "")
			}
			return nil
		}
		current := map[string]string{}
		for _, p := range ps {
			b, _ := json.Marshal(p)
			current[p.ID] = string(b)
			kind := "added"
			if old, ok := previous[p.ID]; ok {
				if old == string(b) {
					continue
				}
				kind = "updated"
			}
			if o.json {
				if err = json.NewEncoder(out).Encode(map[string]any{"event": kind, "provider": p}); err != nil {
					return err
				}
			} else {
				display(out, p, kind+": ")
			}
		}
		for id := range previous {
			if _, ok := current[id]; !ok {
				if o.json {
					if err = json.NewEncoder(out).Encode(map[string]string{"event": "removed", "id": id}); err != nil {
						return err
					}
				} else {
					fmt.Fprintf(out, "removed: %s\n", descriptor.Safe(id))
				}
			}
		}
		previous = current
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}
