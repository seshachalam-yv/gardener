---
globs: "**/*_test.go"
---

# Test Convention Rules

When writing or modifying test files:

1. **Use fake clients (`fakeclient`, `fakekubernetes`) for new tests**, not gomock. Active migration: issue #14572.
2. **No `time.Sleep()`.** Use `Eventually`/`Consistently` from Gomega.
3. **Table-driven tests** with `DescribeTable`/`Entry` for multiple scenarios.
4. **Suite file required** per package: `RegisterFailHandler(Fail)` + `RunSpecs(t, "Suite Name")`.
5. **Check existing test utilities** in `pkg/utils/test/`, `test/utils/`, `test/framework/` before creating duplicates.
6. **Custom matchers** available in `pkg/utils/test/matchers/` (dot-imported): `DeepEqual`, `BeNotFoundError`, etc.
7. **Import convention**: dot-import Ginkgo, Gomega, and custom matchers. Standard alias for fake client: `fakeclient`.
