---
name: api-change
description: Use when modifying Gardener API types (pkg/apis/) — enforces the 8-step checklist from docs/development/changing-the-api.md. Invoked from plan skill when API scope is identified. Do not skip steps.
user-invocable: true
---

# API Change Workflow

## Iron Law

**NO API TYPE MODIFICATION WITHOUT COMPLETING ALL 8 CHECKLIST STEPS IN ORDER.**

| Rationalization | Why it fails |
|---|---|
| "I'll run make generate later" | Protobuf tags must be generated, not copied. Copying tags from another field creates wire-format conflicts that are silent at compile time but break clients. Phase 6: "Do not copy protobuf tags from other fields." |
| "This field doesn't need conversion logic" | Even optional fields may require defaulting (server-side defaults) or conversion (between internal and external versions). Omitting conversion causes fields to be silently dropped during API version negotiation. |
| "I only changed the internal types" | External versions (v1beta1, v1alpha1) must be updated first, then internal. Mismatch causes conversion panics. Step 1 specifies "all external versions and the internal version." |

## Red Flags

- Copying a `protobuf:"bytes,N,..."` tag from another field instead of running `make generate WHAT="protobuf"`
- Adding a new field without pointer type, `// +optional` comment, and `omitempty` JSON tag (all three required)
- Modifying protobuf field numbers on existing fields
- Removing a field in a single step instead of the two-release-cycle process
- Skipping `make generate` after type modifications

## 8-Step Checklist

Follow in order. Do not skip steps.

### Step 1: Modify API types

Add/modify fields in ALL relevant versions:
- External versioned types: `pkg/apis/[group]/[version]/types_[resource].go`
- Internal types: `pkg/apis/[group]/types_[resource].go`

New fields MUST have:
- Pointer type (e.g., `*string`, `*int32`, `*MyType`)
- `// +optional` comment above the field
- `omitempty` in the JSON tag
- Do NOT add protobuf tags manually — they will be generated

### Step 2: Implement/adapt conversion

File: `pkg/apis/[group]/[version]/conversions*.go`

If the field is a simple type match between internal and external, auto-generated conversion handles it. If types differ, write manual conversion functions.

### Step 3: Implement/adapt defaulting

File: `pkg/apis/[group]/[version]/defaults*.go`

If the field needs a default value when not set by the user, add defaulting logic here.

### Step 4: Run code generation

```bash
make generate
```

This runs protobuf, deepcopy, conversion, defaults, OpenAPI, and CRD generation. Verify the generated changes make sense:

```bash
git diff --stat
```

Check that:
- `zz_generated.deepcopy.go` includes the new field
- `zz_generated.defaults.go` includes defaulting if you added it
- `zz_generated.conversion.go` includes conversion if types differ
- CRD YAML in `example/` reflects the new field

### Step 5: Implement/adapt validation

File: `pkg/apis/[group]/validation/validation_[resource].go`

Add validation rules for the new field. Write tests in `validation_[resource]_test.go`.

### Step 6: Update examples

If applicable, update example YAML manifests in `example/` to include the new field.

### Step 7: Update documentation

If applicable, update relevant docs in `docs/`.

### Step 8: Add release note

Include in PR description:

```
` ` `[category] [target_group]
[description of the API change]
` ` `
```

For API changes, typical categories: `feature developer` for new fields, `breaking operator` for removed fields.

## Field Removal Process

Removing an API field is a TWO-release-cycle process:

**Release N:**
1. Remove all code usages of the field
2. Keep the field in API types
3. Add deprecation comment

**Release N+1:**
1. Remove field from all API type versions
2. Tombstone the protobuf number (comment: `// reserved N // was FieldName`)
3. Run `make generate`
4. Add `breaking` release note

**Shoot API defaulted fields require THREE release cycles** (extra cycle for defaulting removal).

## Handoff

All 8 steps complete → invoke implement skill for remaining non-API work, or invoke verify skill if implementation is done.
