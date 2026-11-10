package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yh2237/AviUtl2-MCP/internal/protocol"
)

func TestNormalizeAddObjectProperties(t *testing.T) {
	ops := []protocol.BatchOperation{
		{Op: "add_object", Effect: "図形", Properties: []protocol.PropertyUpdate{{Item: "種類", Value: "円"}, {Effect: "標準描画", Item: "X", Value: "1"}}},
		{Op: "update_object", Properties: []protocol.PropertyUpdate{{Item: "X", Value: "1"}}},
	}
	normalizeBatchOperations(ops)
	if ops[0].Properties[0].Effect != "図形" || ops[0].Properties[1].Effect != "標準描画" {
		t.Fatalf("add_object properties not normalized: %+v", ops[0].Properties)
	}
	if ops[1].Properties[0].Effect != "" {
		t.Fatalf("other ops must not be normalized: %+v", ops[1].Properties)
	}
}

func TestValidateGenericCreateOperations(t *testing.T) {
	layer, frame, negative := 1, 0, -1
	valid := []protocol.BatchOperation{
		{Op: "add_object", Effect: "図形", Layer: &layer, Frame: &frame},
		{Op: "add_from_alias", Alias: "[Object]\n[Object.0]\neffect.name=図形\n", Layer: &layer, Frame: &frame, Length: 30},
	}
	if err := validateBatchOperations(valid); err != nil {
		t.Fatalf("valid operations rejected: %v", err)
	}
	invalid := []protocol.BatchOperation{
		{Op: "add_object", Layer: &layer, Frame: &frame},
		{Op: "add_object", Effect: "図形", Layer: &negative, Frame: &frame},
		{Op: "add_object", Effect: "図形", Layer: &layer, Frame: &frame, Length: -1},
		{Op: "add_from_alias", Layer: &layer, Frame: &frame},
		{Op: "add_from_alias", Alias: "x", Layer: &layer, Frame: &frame, Properties: []protocol.PropertyUpdate{{Effect: "図形", Item: "種類", Value: "円"}}},
	}
	for index, op := range invalid {
		if err := validateBatchOperations([]protocol.BatchOperation{op}); err == nil {
			t.Errorf("invalid[%d] accepted: %+v", index, op)
		}
	}
}

func objectToolArgs(extra map[string]any) map[string]any {
	args := map[string]any{"session_id": "motion-session", "generation": 3, "scene_id": 0, "layer": 0, "frame": 30, "length": 10}
	for key, value := range extra {
		args[key] = value
	}
	return args
}

func sentBatches(t *testing.T, fixture *motionBridgeFixture) []protocol.ExecuteBatchParams {
	t.Helper()
	batches := []protocol.ExecuteBatchParams{}
	for _, request := range fixture.requests() {
		if request.Method != "execute_batch" {
			continue
		}
		var params protocol.ExecuteBatchParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, params)
	}
	return batches
}

func TestAddObjectToolSendsGenericCreate(t *testing.T) {
	fixture := newMotionBridgeFixture()
	fixture.items["図形"] = []protocol.EffectItem{{Name: "種類", Type: 6}, {Name: "サイズ", Type: 2}}
	plan := motionResultPlan(t, callMotionTool(t, fixture, "add_object", objectToolArgs(map[string]any{
		"effect":     "図形",
		"properties": []map[string]string{{"item": "種類", "value": "円"}, {"effect": "標準描画", "item": "X", "value": "100"}},
	})))
	if plan.Mutation == nil {
		t.Fatalf("missing mutation: %+v", plan)
	}
	batches := sentBatches(t, fixture)
	if len(batches) != 1 || len(batches[0].Operations) != 1 {
		t.Fatalf("expected one single-operation batch, got %+v", batches)
	}
	op := batches[0].Operations[0]
	if op.Op != "add_object" || op.Effect != "図形" || *op.Layer != 0 || *op.Frame != 30 || op.Length != 10 {
		t.Fatalf("unexpected operation: %+v", op)
	}
	if len(op.Properties) != 2 || op.Properties[0].Effect != "図形" || op.Properties[1].Effect != "標準描画" {
		t.Fatalf("properties not sent with resolved effects: %+v", op.Properties)
	}
}

func TestAddObjectRejectsBeforeMutation(t *testing.T) {
	cases := []struct {
		name  string
		args  map[string]any
		setup func(*motionBridgeFixture)
		want  string
	}{
		{"unknown effect", map[string]any{"effect": "存在しない"}, func(f *motionBridgeFixture) { f.missingEffects = map[string]bool{"存在しない": true} }, "not available"},
		{"unknown item", map[string]any{"effect": "図形", "properties": []map[string]string{{"item": "存在しない項目", "value": "1"}}}, func(f *motionBridgeFixture) { f.items["図形"] = []protocol.EffectItem{{Name: "種類"}} }, "has no item"},
		// 既存object 7 はレイヤー2のフレーム10-19にある
		{"overlap", map[string]any{"effect": "図形", "layer": 2, "frame": 15, "length": 10}, func(*motionBridgeFixture) {}, "operations[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newMotionBridgeFixture()
			tc.setup(fixture)
			result := callMotionTool(t, fixture, "add_object", objectToolArgs(tc.args))
			if !result.IsError {
				t.Fatalf("expected an error, got %+v", result.StructuredContent)
			}
			encoded, _ := json.Marshal(result.Content)
			if !strings.Contains(string(encoded), tc.want) {
				t.Fatalf("error %s does not mention %q", encoded, tc.want)
			}
			if batches := sentBatches(t, fixture); len(batches) != 0 {
				t.Fatalf("mutation was sent despite preflight failure: %+v", batches)
			}
		})
	}
}

func TestAddFromAliasToolSendsAlias(t *testing.T) {
	fixture := newMotionBridgeFixture()
	alias := "[Object]\n[Object.0]\neffect.name=図形\n種類=円\n"
	motionResultPlan(t, callMotionTool(t, fixture, "add_from_alias", objectToolArgs(map[string]any{"alias": alias, "length": 0})))
	batches := sentBatches(t, fixture)
	if len(batches) != 1 || batches[0].Operations[0].Op != "add_from_alias" || batches[0].Operations[0].Alias != alias {
		t.Fatalf("alias not sent: %+v", batches)
	}
}
