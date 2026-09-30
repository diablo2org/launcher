// Package spec parses and checks the documents servers publish: the server
// profile and file manifests. Every document is first checked
// against its JSON Schema from the schema folder, then against the rules a
// schema can't express, such as references between fields.
package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/d2org/launcher/schema"
)

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

const (
	profileSchemaID  = "https://diablo2.org/schema/server-profile/1.json"
	manifestSchemaID = "https://diablo2.org/schema/file-manifest/1.json"
)

// schemas compiles the embedded schemas the first time they're needed. A
// failure here means a broken schema file, which the tests catch.
func schemas() (map[string]*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		sources := map[string][]byte{
			profileSchemaID:  schema.ServerProfile,
			manifestSchemaID: schema.FileManifest,
		}

		c := jsonschema.NewCompiler()
		for id, source := range sources {
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(source))
			if err != nil {
				compileErr = fmt.Errorf("schema %s: %w", id, err)
				return
			}

			if err := c.AddResource(id, doc); err != nil {
				compileErr = fmt.Errorf("schema %s: %w", id, err)
				return
			}
		}

		compiled = make(map[string]*jsonschema.Schema, len(sources))
		for id := range sources {
			s, err := c.Compile(id)
			if err != nil {
				compileErr = fmt.Errorf("schema %s: %w", id, err)
				return
			}

			compiled[id] = s
		}
	})

	return compiled, compileErr
}

// decode checks data against the schema with the given id, then unmarshals
// it into v.
func decode(schemaID string, data []byte, v interface{}) error {
	all, err := schemas()
	if err != nil {
		return err
	}

	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}

	if err := all[schemaID].Validate(instance); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return schemaProblems(ve)
		}

		return err
	}

	return json.Unmarshal(data, v)
}

// schemaProblems flattens a schema validation error into one line per failed
// rule, each saying where in the document it failed.
func schemaProblems(ve *jsonschema.ValidationError) Problems {
	printer := message.NewPrinter(language.English)

	var problems Problems
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}

		at := "/" + strings.Join(e.InstanceLocation, "/")
		problems.add("at %s: %s", at, e.ErrorKind.LocalizedString(printer))
	}
	walk(ve)

	return problems
}

// Problems collects everything wrong with a document, so a server author sees
// every issue at once rather than one per attempt.
type Problems []string

func (p *Problems) add(format string, args ...interface{}) {
	*p = append(*p, fmt.Sprintf(format, args...))
}

func (p Problems) Error() string {
	return strings.Join(p, "\n")
}

// err returns nil when nothing was found.
func (p Problems) err() error {
	if len(p) == 0 {
		return nil
	}

	return p
}

// allowedHost reports whether rawURL is https on one of hosts.
func allowedHost(rawURL string, hosts []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}

	host := strings.ToLower(u.Hostname())
	for _, h := range hosts {
		if host == h {
			return true
		}
	}

	return false
}
