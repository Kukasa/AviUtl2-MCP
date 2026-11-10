package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yh2237/AviUtl2-MCP/internal/bridge"
	"github.com/yh2237/AviUtl2-MCP/internal/protocol"
)

// objectPropertyInput mirrors protocol.PropertyUpdate but lets effect be omitted,
// so the generated input schema does not require it.
type objectPropertyInput struct {
	Effect string `json:"effect,omitempty" jsonschema:"effect that owns the item; omit to target the created object's own effect"`
	Item   string `json:"item" jsonschema:"item name, see list_effects with effect"`
	Value  string `json:"value" jsonschema:"item value as text"`
}

type addObjectInput struct {
	mutationContext
	Effect     string                `json:"effect" jsonschema:"effect name used as the object's first effect (the alias effect.name), e.g. 図形, シーン, or a filter effect name such as ぼかし for a filter object; see list_effects"`
	Layer      int                   `json:"layer" jsonschema:"zero-based target layer"`
	Frame      int                   `json:"frame" jsonschema:"zero-based start frame"`
	Length     int                   `json:"length,omitempty" jsonschema:"duration in frames; 0 lets AviUtl2 choose the length and position"`
	Properties []objectPropertyInput `json:"properties,omitempty" jsonschema:"initial item values, e.g. {item:種類,value:円}; omit effect to target the created object's own effect"`
	DryRun     bool                  `json:"dry_run,omitempty"`
}

type addFromAliasInput struct {
	mutationContext
	Alias  string `json:"alias" jsonschema:"object alias text in .object file format ([Object] / [Object.0] / effect.name=...)"`
	Layer  int    `json:"layer" jsonschema:"zero-based target layer"`
	Frame  int    `json:"frame" jsonschema:"zero-based start frame"`
	Length int    `json:"length,omitempty" jsonschema:"duration in frames; 0 lets AviUtl2 decide; frame data inside the alias takes precedence"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// normalizeBatchOperations fills in implicit values before validation.
// For add_object, a property without an effect targets the created object's own effect.
func normalizeBatchOperations(operations []protocol.BatchOperation) {
	for index := range operations {
		operation := &operations[index]
		if operation.Op != "add_object" {
			continue
		}
		for propertyIndex := range operation.Properties {
			if strings.TrimSpace(operation.Properties[propertyIndex].Effect) == "" {
				operation.Properties[propertyIndex].Effect = operation.Effect
			}
		}
	}
}

func addObjectTools(server *mcp.Server, client *bridge.Client) {
	addTool(server, &mcp.Tool{Name: "add_object", Description: "Create any object by effect name, such as a shape (図形), a scene object (シーン), or a filter object (a filter effect name like ぼかし), and set initial item values in one Undo unit. Use list_effects to find effect and item names. Also available as op add_object in execute_batch, where an empty properties effect (empty string) targets the created object's own effect."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input addObjectInput) (*mcp.CallToolResult, operationPlanOutput, error) {
			if err := input.mutationContext.validate(); err != nil {
				return nil, operationPlanOutput{}, err
			}
			layer, frame := input.Layer, input.Frame
			properties := make([]protocol.PropertyUpdate, 0, len(input.Properties))
			for _, property := range input.Properties {
				properties = append(properties, protocol.PropertyUpdate(property))
			}
			ops := []protocol.BatchOperation{{Op: "add_object", Effect: input.Effect, Layer: &layer, Frame: &frame, Length: input.Length, Properties: properties}}
			normalizeBatchOperations(ops)
			return executeOperationPlan(ctx, client, input.mutationContext, ops, input.DryRun, true)
		})

	addTool(server, &mcp.Tool{Name: "add_from_alias", Description: "Create object(s) from alias text in .object file format. Use this when add_object cannot express the object; inspect_object with include_alias returns an existing object's alias to use as a template. Also available as op add_from_alias in execute_batch."},
		func(ctx context.Context, _ *mcp.CallToolRequest, input addFromAliasInput) (*mcp.CallToolResult, operationPlanOutput, error) {
			if err := input.mutationContext.validate(); err != nil {
				return nil, operationPlanOutput{}, err
			}
			layer, frame := input.Layer, input.Frame
			ops := []protocol.BatchOperation{{Op: "add_from_alias", Alias: input.Alias, Layer: &layer, Frame: &frame, Length: input.Length}}
			return executeOperationPlan(ctx, client, input.mutationContext, ops, input.DryRun, true)
		})
}
