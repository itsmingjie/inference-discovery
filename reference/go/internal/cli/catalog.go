package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/inference"
)

type catalogBuilder struct {
	cfg     advertiseConfig
	api     inference.Client
	timeout time.Duration
	out     io.Writer
	checked map[string]modelCheck
}

type modelCheck struct {
	api   *descriptor.ModelAPI
	retry time.Time // Inconclusive checks can recover without restarting the advertiser.
}

func (b *catalogBuilder) detect(ctx context.Context, models []descriptor.Model) []descriptor.Model {
	now := time.Now()
	next := make(map[string]modelCheck, len(models))
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	results := make([]*descriptor.ModelAPI, len(models))
	problems := make([]error, len(models))
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i, m := range models {
		if m.API != nil {
			results[i] = m.API
			continue
		}
		if check, ok := b.checked[m.ID]; ok && (check.api != nil || now.Before(check.retry)) {
			results[i] = check.api
			next[m.ID] = check
			continue
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				problems[i] = ctx.Err()
				return
			}
			defer func() { <-slots }()
			api, err := b.api.Detect(ctx, m.ID)
			if err != nil {
				problems[i] = err
				return
			}
			results[i] = &api
		}()
	}
	workers.Wait()
	catalog := make([]descriptor.Model, 0, len(models))
	for i, m := range models {
		if _, checked := next[m.ID]; !checked && m.API == nil {
			next[m.ID] = modelCheck{api: results[i], retry: time.Now().Add(time.Minute)}
			if problems[i] != nil && b.out != nil {
				fmt.Fprintf(b.out, "Model %s skipped: %s\n", descriptor.Safe(m.ID), descriptor.Safe(problems[i].Error()))
			}
		}
		if results[i] != nil {
			m.API = results[i]
			catalog = append(catalog, m)
		}
	}
	b.checked = next // Forget models no longer listed, bounding the cache as models change.
	return catalog
}

// build refreshes availability, retaining capability checks for known model IDs.
func (builder *catalogBuilder) build(ctx context.Context) ([]byte, error) {
	cfg, api := builder.cfg, builder.api
	d := descriptor.Descriptor{
		Version: 1,
		Name:    cfg.Name,
		API:     descriptor.API{BaseURL: cfg.Endpoint},
		Auth:    descriptor.Auth{Methods: []string{"none"}},
	}
	models, err := api.Catalog(ctx)
	if err != nil {
		return nil, err
	}
	catalog := make([]descriptor.Model, 0, len(models))
	if cfg.Models == nil {
		catalog = models
	} else {
		for _, m := range cfg.Models {
			if slices.ContainsFunc(models, func(available descriptor.Model) bool { return available.ID == m.ID }) {
				catalog = append(catalog, m)
			}
		}
	}
	if len(catalog) > 128 {
		return nil, fmt.Errorf("catalog exceeds 128 models; configure a smaller model list")
	}
	catalog = builder.detect(ctx, catalog)
	if len(catalog) == 0 {
		return nil, fmt.Errorf("no usable models; see diagnostics or configure model API metadata")
	}
	// Common claims remain safe for clients that ignore per-model extensions.
	d.API.Profiles = append([]string{}, catalog[0].API.Profiles...)
	d.API.Capabilities = append([]string{}, catalog[0].API.Capabilities...)
	for _, m := range catalog[1:] {
		d.API.Profiles = slices.DeleteFunc(d.API.Profiles, func(p string) bool { return !slices.Contains(m.API.Profiles, p) })
		d.API.Capabilities = slices.DeleteFunc(d.API.Capabilities, func(c string) bool { return !slices.Contains(m.API.Capabilities, c) })
	}
	for i, m := range catalog {
		if slices.Equal(m.API.Profiles, d.API.Profiles) && slices.Equal(m.API.Capabilities, d.API.Capabilities) {
			catalog[i].API = nil
		}
	}
	ids := make([]string, 0, len(catalog))
	for _, m := range catalog {
		ids = append(ids, m.ID)
	}
	slices.Sort(ids)
	model, err := inference.SelectModel(ids, "", cfg.Model)
	if err != nil {
		return nil, err
	}
	d.API.DefaultModel = model
	d.API.Models = catalog
	if err := d.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(d)
	if len(b) > descriptor.MaxBytes {
		return nil, fmt.Errorf("descriptor exceeds 32 KiB; configure a smaller model list")
	}
	return b, err
}
