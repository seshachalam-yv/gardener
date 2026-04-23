---
globs: pkg/apis/**/*.go
---

# API Type Rules

When modifying files in `pkg/apis/`:

1. **New fields must be optional**: pointer type + `// +optional` comment + `omitempty` JSON tag. All three are required.
2. **Do not copy protobuf tags** from other fields. Run `make generate WHAT="protobuf"` to generate them.
3. **Run `make generate`** after ANY type modification, then `make check-generate` to verify.
4. **Follow the 8-step checklist** in `docs/development/changing-the-api.md` for all API changes.
5. **Field removal is a two-release-cycle process.** Do not remove fields in a single step.
6. **Internal and external types must stay in sync.** Update all versions (internal, v1beta1, v1alpha1) together.
7. **Validation lives in `pkg/apis/[group]/validation/`**, not alongside type definitions.
8. **Constants-only changes** (adding annotation keys, label keys, or other string constants without struct field changes) do NOT require steps 2-6 of the API change checklist. Only step 1 (add constant), step 7 (docs), and step 8 (release note) apply.
