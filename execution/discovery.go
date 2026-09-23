package execution

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/anurgosw/agentic-moe/policy"
)

// CapabilityQuery pages through the policy-filtered catalog. Schemas are
// opt-in so a large tool registry does not consume every provider request.
type CapabilityQuery struct {
	ToolSet        string `json:"tool_set,omitempty"`
	Search         string `json:"search,omitempty"`
	Cursor         string `json:"cursor,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	IncludeSchemas bool   `json:"include_schemas,omitempty"`
	MaxSchemaBytes int    `json:"max_schema_bytes,omitempty"`
}

type CapabilitySummary struct {
	Ref           ToolRef         `json:"ref"`
	Description   string          `json:"description"`
	SchemaDigest  string          `json:"schema_digest"`
	Class         ToolClass       `json:"class"`
	Schema        json.RawMessage `json:"schema,omitempty"`
	SchemaOmitted bool            `json:"schema_omitted,omitempty"`
}

type CapabilityPage struct {
	CatalogVersion string              `json:"catalog_version"`
	Items          []CapabilitySummary `json:"items"`
	NextCursor     string              `json:"next_cursor,omitempty"`
}

// Discover returns a deterministic page tied to this admitted catalog
// generation. A cursor from another generation fails instead of drifting.
func (s CatalogSnapshot) Discover(query CapabilityQuery, snapshot policy.Snapshot) (CapabilityPage, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		return CapabilityPage{}, fmt.Errorf("capability page limit exceeds 100")
	}
	offset, err := decodeCapabilityCursor(query.Cursor, s.version)
	if err != nil {
		return CapabilityPage{}, err
	}
	needle := strings.ToLower(strings.TrimSpace(query.Search))
	all := s.Capabilities(query.ToolSet, snapshot)
	filtered := make([]Capability, 0, len(all))
	for _, capability := range all {
		if needle != "" && !strings.Contains(strings.ToLower(capability.Ref.Name+" "+capability.Description), needle) {
			continue
		}
		filtered = append(filtered, capability)
	}
	if offset > len(filtered) {
		return CapabilityPage{}, fmt.Errorf("capability cursor is past the catalog end")
	}
	page := CapabilityPage{CatalogVersion: s.version, Items: make([]CapabilitySummary, 0, limit)}
	schemaBytes := 0
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	for _, capability := range filtered[offset:end] {
		item := CapabilitySummary{Ref: capability.Ref, Description: capability.Description, SchemaDigest: capability.SchemaDigest, Class: capability.Class}
		if query.IncludeSchemas {
			if query.MaxSchemaBytes > 0 && schemaBytes+len(capability.Schema) > query.MaxSchemaBytes {
				item.SchemaOmitted = true
			} else {
				item.Schema = append(json.RawMessage(nil), capability.Schema...)
				schemaBytes += len(capability.Schema)
			}
		}
		page.Items = append(page.Items, item)
	}
	if end < len(filtered) {
		page.NextCursor = encodeCapabilityCursor(s.version, end)
	}
	return page, nil
}

func encodeCapabilityCursor(version string, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(version + "\x00" + strconv.Itoa(offset)))
}

func decodeCapabilityCursor(cursor, version string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("decode capability cursor: %w", err)
	}
	parts := strings.Split(string(payload), "\x00")
	if len(parts) != 2 || parts[0] != version {
		return 0, fmt.Errorf("capability cursor does not match admitted catalog")
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid capability cursor offset")
	}
	return offset, nil
}
