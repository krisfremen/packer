package hcl2template

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/packer/packer"
)

// SourceBlock references an HCL 'source' block.
type SourceBlock struct {
	// Type of source; ex: virtualbox-iso
	Type string
	// Given name; if any
	Name string

	block *hcl.Block

	// addition will be merged into block to allow user to override builder settings
	// per build.source block.
	addition hcl.Body
}

// decodeBuildSource reads a used source block from a build:
//  build {
//   source "type.example" {}
//  }
func (p *Parser) decodeBuildSource(block *hcl.Block) (SourceRef, hcl.Diagnostics) {
	ref := sourceRefFromString(block.Labels[0])
	ref.addition = block.Body
	return ref, nil
}

func (p *Parser) decodeSource(block *hcl.Block) (SourceBlock, hcl.Diagnostics) {
	source := SourceBlock{
		Type:  block.Labels[0],
		Name:  block.Labels[1],
		block: block,
	}
	var diags hcl.Diagnostics

	if !p.BuilderSchemas.Has(source.Type) {
		diags = append(diags, &hcl.Diagnostic{
			Summary:  "Unknown " + buildSourceLabel + " type " + source.Type,
			Subject:  block.LabelRanges[0].Ptr(),
			Detail:   fmt.Sprintf("known builders: %v", p.BuilderSchemas.List()),
			Severity: hcl.DiagError,
		})
		return source, diags
	}

	return source, diags
}

func (cfg *PackerConfig) startBuilder(source SourceBlock, ectx *hcl.EvalContext, opts packer.GetBuildsOptions) (packer.Builder, hcl.Diagnostics, []string) {
	var diags hcl.Diagnostics

	builder, err := cfg.builderSchemas.Start(source.Type)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Summary: "Failed to load " + sourceLabel + " type",
			Detail:  err.Error(),
			Subject: &source.block.LabelRanges[0],
		})
		return builder, diags, nil
	}

	body := source.block.Body
	if source.addition != nil {
		body = hcl.MergeBodies([]hcl.Body{source.block.Body, source.addition})
	}

	decoded, moreDiags := decodeHCL2Spec(body, ectx, builder)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return nil, diags, nil
	}

	// Note: HCL prepares inside of the Start func, but Json does not. Json
	// builds are instead prepared only in command/build.go
	// TODO: either make json prepare when plugins are loaded, or make HCL
	// prepare at a later step, to make builds from different template types
	// easier to reason about.
	builderVars := source.builderVariables()
	builderVars["packer_debug"] = strconv.FormatBool(opts.Debug)
	builderVars["packer_force"] = strconv.FormatBool(opts.Force)
	builderVars["packer_on_error"] = opts.OnError

	generatedVars, warning, err := builder.Prepare(builderVars, decoded)
	moreDiags = warningErrorsToDiags(source.block, warning, err)
	diags = append(diags, moreDiags...)
	return builder, diags, generatedVars
}

// These variables will populate the PackerConfig inside of the builders.
func (source *SourceBlock) builderVariables() map[string]string {
	return map[string]string{
		"packer_build_name":   source.Name,
		"packer_builder_type": source.Type,
	}
}

func (source *SourceBlock) Ref() SourceRef {
	return SourceRef{
		Type: source.Type,
		Name: source.Name,
	}
}

type SourceRef struct {
	Type string
	Name string

	// The content of this body will be merged into a new block to allow to
	// override builder settings per build section.
	addition hcl.Body
}

// the 'addition' field makes of ref a different entry in the sources map, so
// Ref is here to make sure only one is returned.
func (r *SourceRef) Ref() SourceRef {
	return SourceRef{
		Type: r.Type,
		Name: r.Name,
	}
}

// NoSource is the zero value of sourceRef, representing the absense of an
// source.
var NoSource SourceRef

func (r SourceRef) String() string {
	return fmt.Sprintf("%s.%s", r.Type, r.Name)
}
