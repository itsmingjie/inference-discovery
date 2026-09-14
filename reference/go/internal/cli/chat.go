package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/authentication"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/inference"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/transport"
)

func chat(ctx context.Context, selected provider, opts options, reader *bufio.Scanner, out io.Writer) error {
	d := *selected.Descriptor
	session, err := authentication.None{}.Resolve(ctx, d)
	if err != nil {
		return err
	}
	if err = d.Compatible(false); err != nil {
		return err
	}
	models := make([]string, 0, len(d.API.Models))
	streaming := make(map[string]bool, len(d.API.Models))
	for _, m := range d.API.Models {
		if err := d.ModelCompatible(m, false); err == nil {
			models = append(models, m.ID)
			streaming[m.ID] = slices.Contains(d.APIFor(m).Capabilities, "streaming")
		} else if m.ID == opts.model {
			return err
		}
	}
	slices.Sort(models)
	def := d.API.DefaultModel
	if !slices.Contains(models, def) {
		def = ""
	}
	model, err := inference.SelectModel(models, def, opts.model)
	if err != nil {
		return err
	}
	api := inference.Client{HTTP: transport.Client(opts.chatTimeout), Base: d.API.BaseURL, Session: session}
	defer api.HTTP.CloseIdleConnections()
	fmt.Fprintf(out, "Connected to %s. Model: %s\n", descriptor.Safe(d.Name), descriptor.Safe(model))
	if len(models) > 1 {
		fmt.Fprintf(out, "Available models: %s (select with --model).\n", descriptor.Safe(strings.Join(models, ", ")))
	}
	fmt.Fprintln(out, "Discovery does not establish identity or guarantee local processing. /quit exits.")
	history := []inference.Message{}
	for {
		fmt.Fprint(out, "\nYou: ")
		if !reader.Scan() {
			return reader.Err()
		}
		prompt := reader.Text()
		if prompt == "/quit" {
			return nil
		}
		if strings.TrimSpace(prompt) == "" {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		next := append(history, inference.Message{Role: "user", Content: prompt})
		fmt.Fprint(out, "Assistant: ")
		answer, err := api.Chat(ctx, model, next, streaming[model], func(s string) error { _, e := io.WriteString(out, descriptor.Safe(s)); return e })
		fmt.Fprintln(out)
		if err != nil {
			return fmt.Errorf("conversation stopped; no retry or provider switch: %w", err)
		}
		history = append(next, inference.Message{Role: "assistant", Content: answer})
	}
}
