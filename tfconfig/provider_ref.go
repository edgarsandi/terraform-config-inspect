package tfconfig

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// ProviderRef is a reference to a provider configuration within a module.
// It represents the contents of a "provider" argument in a resource, or
// a value in the "providers" map for a module call.
type ProviderRef struct {
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"` // Empty if the default provider configuration is referenced
}

type ProviderRequirement struct {
	Source               string        `json:"source,omitempty"`
	VersionConstraints   []string      `json:"version_constraints,omitempty"`
	ConfigurationAliases []ProviderRef `json:"aliases,omitempty"`
}

func decodeRequiredProvidersBlock(block *hcl.Block) (map[string]*ProviderRequirement, hcl.Diagnostics) {
	attrs, diags := block.Body.JustAttributes()
	reqs := make(map[string]*ProviderRequirement)
	for name, attr := range attrs {
		// Look for a legacy version in the attribute first
		if expr, err := attr.Expr.Value(nil); err == nil && expr.Type().IsPrimitiveType() {
			var version string
			valDiags := gohcl.DecodeExpression(attr.Expr, nil, &version)
			diags = append(diags, valDiags...)
			if !valDiags.HasErrors() {
				reqs[name] = &ProviderRequirement{
					VersionConstraints: []string{version},
				}
			}
			continue
		}

		kvs, mapDiags := hcl.ExprMap(attr.Expr)
		if mapDiags.HasErrors() {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid required_providers object",
				Detail:   "Required providers entries must be strings or objects.",
				Subject:  attr.Expr.Range().Ptr(),
			})
			continue
		}

		var pr ProviderRequirement

		for _, kv := range kvs {
			key, keyDiags := kv.Key.Value(nil)
			if keyDiags.HasErrors() {
				diags = append(diags, keyDiags...)
				continue
			}

			if key.Type() != cty.String {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Invalid Attribute",
					Detail:   fmt.Sprintf("Invalid attribute value for provider requirement: %#v", key),
					Subject:  kv.Key.Range().Ptr(),
				})
				continue
			}

			switch key.AsString() {
			case "version":
				version, valDiags := kv.Value.Value(nil)
				if valDiags.HasErrors() || !version.Type().Equals(cty.String) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Unsuitable value type",
						Detail:   "Unsuitable value: string required",
						Subject:  attr.Expr.Range().Ptr(),
					})
					continue
				}
				if !version.IsNull() {
					pr.VersionConstraints = append(pr.VersionConstraints, version.AsString())
				}

			case "source":
				source, valDiags := kv.Value.Value(nil)
				if valDiags.HasErrors() || !source.Type().Equals(cty.String) {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Unsuitable value type",
						Detail:   "Unsuitable value: string required",
						Subject:  attr.Expr.Range().Ptr(),
					})
					continue
				}

				if !source.IsNull() {
					pr.Source = source.AsString()
				}
			case "configuration_aliases":
				aliases, valDiags := decodeConfigurationAliases(kv.Value)
				if valDiags.HasErrors() {
					diags = append(diags, valDiags...)
					continue
				}
				pr.ConfigurationAliases = append(pr.ConfigurationAliases, aliases...)
			}

			reqs[name] = &pr
		}
	}

	return reqs, diags
}

func decodeConfigurationAliases(value hcl.Expression) ([]ProviderRef, hcl.Diagnostics) {
	aliases := make([]ProviderRef, 0)
	var diags hcl.Diagnostics

	tuple, ok := value.(*hclsyntax.TupleConsExpr)
	if !ok {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Unsuitable value type",
			Detail:   "Unsuitable value: tuple required",
			Subject:  value.Range().Ptr(),
		})
		return aliases, diags
	}

	for _, expr := range tuple.Exprs {
		alias, ok := expr.(*hclsyntax.ScopeTraversalExpr)
		if !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unsuitable value type for alias",
				Detail:   "Unsuitable type of alias: traversal required",
				Subject:  expr.Range().Ptr(),
			})
			continue
		}

		traversal := alias.AsTraversal()
		if len(traversal) != 2 {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unexpected alias identification",
				Detail:   "Expected identification in localname.alias format",
				Subject:  expr.Range().Ptr(),
			})
			continue
		}

		tAttr, ok := traversal[1].(hcl.TraverseAttr)
		if !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Unexpected alias identification",
				Detail:   "Expected second part of identification to be the alias (string)",
				Subject:  expr.Range().Ptr(),
			})
			continue
		}

		aliases = append(aliases, ProviderRef{
			Name:  traversal.RootName(),
			Alias: tAttr.Name,
		})
	}

	return aliases, diags
}
