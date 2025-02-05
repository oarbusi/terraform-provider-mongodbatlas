package acc

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/mongodb/terraform-provider-mongodbatlas/internal/config"
	"github.com/mongodb/terraform-provider-mongodbatlas/internal/testutil/hcl"
	"github.com/zclconf/go-cty/cty"

	"github.com/stretchr/testify/assert"
)

func CheckRSAndDSSchemaV2(isAcc bool, resourceName string, dataSourceName, pluralDataSourceName *string, attrsSet []string, attrsMap map[string]string, extra ...resource.TestCheckFunc) resource.TestCheckFunc {
	modifiedSet := ConvertToSchemaV2AttrsSet(isAcc, attrsSet)
	modifiedMap := ConvertToSchemaV2AttrsMap(isAcc, attrsMap)
	return CheckRSAndDS(resourceName, dataSourceName, pluralDataSourceName, modifiedSet, modifiedMap, extra...)
}

func TestCheckResourceAttrSchemaV2(isAcc bool, name, key, value string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttr(name, AttrNameToSchemaV2(isAcc, key), value)
}

func TestCheckResourceAttrSetSchemaV2(isAcc bool, name, key string) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrSet(name, AttrNameToSchemaV2(isAcc, key))
}

func TestCheckResourceAttrWithSchemaV2(isAcc bool, name, key string, checkValueFunc resource.CheckResourceAttrWithFunc) resource.TestCheckFunc {
	return resource.TestCheckResourceAttrWith(name, AttrNameToSchemaV2(isAcc, key), checkValueFunc)
}

func TestCheckTypeSetElemNestedAttrsSchemaV2(isAcc bool, name, key string, values map[string]string) resource.TestCheckFunc {
	return resource.TestCheckTypeSetElemNestedAttrs(name, AttrNameToSchemaV2(isAcc, key), values)
}

func AddAttrChecksSchemaV2(isAcc bool, name string, checks []resource.TestCheckFunc, mapChecks map[string]string) []resource.TestCheckFunc {
	return AddAttrChecks(name, checks, ConvertToSchemaV2AttrsMap(isAcc, mapChecks))
}

func AddAttrSetChecksSchemaV2(isAcc bool, name string, checks []resource.TestCheckFunc, attrNames ...string) []resource.TestCheckFunc {
	return AddAttrSetChecks(name, checks, ConvertToSchemaV2AttrsSet(isAcc, attrNames)...)
}

func AddAttrChecksPrefixSchemaV2(isAcc bool, name string, checks []resource.TestCheckFunc, mapChecks map[string]string, prefix string, skipNames ...string) []resource.TestCheckFunc {
	return AddAttrChecksPrefix(name, checks, ConvertToSchemaV2AttrsMap(isAcc, mapChecks), prefix, skipNames...)
}

func ConvertToSchemaV2AttrsMap(isAcc bool, attrsMap map[string]string) map[string]string {
	if skipSchemaV2Work(isAcc) {
		return attrsMap
	}
	ret := make(map[string]string, len(attrsMap))
	for name, value := range attrsMap {
		ret[AttrNameToSchemaV2(isAcc, name)] = value
	}
	return ret
}

func ConvertToSchemaV2AttrsSet(isAcc bool, attrsSet []string) []string {
	if skipSchemaV2Work(isAcc) {
		return attrsSet
	}
	ret := make([]string, 0, len(attrsSet))
	for _, name := range attrsSet {
		ret = append(ret, AttrNameToSchemaV2(isAcc, name))
	}
	return ret
}

var tpfSingleNestedAttrs = []string{
	"analytics_specs",
	"electable_specs",
	"read_only_specs",
	"auto_scaling", // includes analytics_auto_scaling
	"advanced_configuration",
	"bi_connector_config",
	"pinned_fcv",
	"timeouts",
	"connection_strings",
	"tags",
}

func AttrNameToSchemaV2(isAcc bool, name string) string {
	if skipSchemaV2Work(isAcc) {
		return name
	}
	for _, singleAttrName := range tpfSingleNestedAttrs {
		name = strings.ReplaceAll(name, singleAttrName+".0", singleAttrName)
	}
	return name
}

func ConvertAdvancedClusterToSchemaV2(t *testing.T, isAcc bool, def string) string {
	t.Helper()
	if skipSchemaV2Work(isAcc) {
		return def
	}
	parse := hcl.GetDefParser(t, def)
	for _, resource := range parse.Body().Blocks() {
		isResource := resource.Type() == "resource"
		resourceName := resource.Labels()[0]
		if !isResource || resourceName != "mongodbatlas_advanced_cluster" {
			continue
		}
		writeBody := resource.Body()
		convertAttrs(t, "replication_specs", writeBody, true, getReplicationSpecs)
		convertAttrs(t, "advanced_configuration", writeBody, false, hcl.GetAttrVal)
		convertAttrs(t, "bi_connector_config", writeBody, false, hcl.GetAttrVal)
		convertAttrs(t, "pinned_fcv", writeBody, false, hcl.GetAttrVal)
		convertAttrs(t, "timeouts", writeBody, false, hcl.GetAttrVal)
		convertKeyValueAttrs(t, "labels", writeBody)
		convertKeyValueAttrs(t, "tags", writeBody)
	}
	content := parse.Bytes()
	return string(content)
}

func skipSchemaV2Work(isAcc bool) bool {
	return !config.AdvancedClusterV2Schema() || !isAcc
}

func AssertEqualHCL(t *testing.T, expected, actual string, msgAndArgs ...interface{}) {
	t.Helper()
	assert.Equal(t, hcl.CanonicalHCL(t, expected), hcl.CanonicalHCL(t, actual), msgAndArgs...)
}

func convertAttrs(t *testing.T, name string, writeBody *hclwrite.Body, isList bool, getOneAttr func(*testing.T, *hclsyntax.Body) cty.Value) {
	t.Helper()
	var vals []cty.Value
	for {
		match := writeBody.FirstMatchingBlock(name, nil)
		if match == nil {
			break
		}
		vals = append(vals, getOneAttr(t, hcl.GetBlockBody(t, match)))
		writeBody.RemoveBlock(match) // TODO: RemoveBlock doesn't remove newline just after the block so an extra line is added
	}
	if len(vals) == 0 {
		return
	}
	if isList {
		writeBody.SetAttributeValue(name, cty.TupleVal(vals))
	} else {
		assert.Len(t, vals, 1, "can be only one of %s", name)
		writeBody.SetAttributeValue(name, vals[0])
	}
}

func convertKeyValueAttrs(t *testing.T, name string, writeBody *hclwrite.Body) {
	t.Helper()
	vals := make(map[string]cty.Value)
	for {
		match := writeBody.FirstMatchingBlock(name, nil)
		if match == nil {
			break
		}
		attrs := hcl.GetAttrVal(t, hcl.GetBlockBody(t, match))
		key := attrs.GetAttr("key")
		value := attrs.GetAttr("value")
		vals[key.AsString()] = value
		writeBody.RemoveBlock(match) // TODO: RemoveBlock doesn't remove newline just after the block so an extra line is added
	}
	if len(vals) > 0 {
		writeBody.SetAttributeValue(name, cty.ObjectVal(vals))
	}
}

func getReplicationSpecs(t *testing.T, body *hclsyntax.Body) cty.Value {
	t.Helper()
	const name = "region_configs"
	var vals []cty.Value
	for _, block := range body.Blocks {
		assert.Equal(t, name, block.Type, "unexpected block type: %s", block.Type)
		vals = append(vals, hcl.GetAttrVal(t, block.Body))
	}
	attributeValues := map[string]cty.Value{
		name: cty.TupleVal(vals),
	}
	hcl.AddAttributes(t, body, attributeValues)
	return cty.ObjectVal(attributeValues)
}
