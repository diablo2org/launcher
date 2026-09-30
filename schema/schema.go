// Package schema embeds the JSON Schemas that define the documents servers
// publish, so the launcher validates against exactly the files in this folder.
package schema

import _ "embed"

// ServerProfile is schema/server-profile.schema.json.
//
//go:embed server-profile.schema.json
var ServerProfile []byte

// FileManifest is schema/file-manifest.schema.json.
//
//go:embed file-manifest.schema.json
var FileManifest []byte
