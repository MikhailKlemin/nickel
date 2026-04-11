# Go 1.25+ Modern & Idiomatic Code Guidelines

You are an expert Go developer. When generating, reviewing, or modifying Go code, you MUST follow these rules unconditionally. The target Go version is **1.25** (or later). Never produce code that would be flagged by `go fix`, `gopls/modernize`, or `go vet`.

---

## 1. Language Version & Module

- Always assume `go 1.25` or higher in `go.mod`.
- Use `//go:build go1.25` build constraints in files that rely on Go 1.25+ features.
- Do not suggest downgrading language features for "compatibility" unless explicitly asked.

---

## 2. Type Aliases & Built-ins

| Old / Pre-1.18 style | Modern Go 1.18+ style |
|---|---|
| `interface{}` | `any` |
| manual `min`/`max` if-else | `min(a, b)` / `max(a, b)` (Go 1.21) |
| `new(T); *ptr = val` (two lines) | `new(val)` (Go 1.26 — use if go.mod ≥ 1.26) |

Always use `any` instead of `interface{}`. Always use the built-in `min`/`max` instead of conditional assignments.

---

## 3. Slices — Use the `slices` Package (Go 1.21+)

Never write manual slice loops when a `slices` package function exists.

| Avoid | Prefer |
|---|---|
| `append([]T(nil), s...)` | `slices.Clone(s)` |
| `append(s1, s2...)` for copy | `slices.Concat(s1, s2)` |
| manual `sort.Slice(s, func(i,j int) bool {...})` | `slices.Sort(s)` / `slices.SortFunc(s, cmp)` |
| manual index removal | `slices.Delete(s, i, i+1)` |
| manual linear search | `slices.Contains(s, v)` / `slices.Index(s, v)` |
| `reflect.DeepEqual` for slices | `slices.Equal(a, b)` |

---

## 4. Maps — Use the `maps` Package (Go 1.21+)

| Avoid | Prefer |
|---|---|
| Manual copy loop over map | `maps.Copy(dst, src)` |
| Manual clone loop | `maps.Clone(m)` |
| Manual collect from iter | `maps.Collect(iter)` |
| Manual insert loop | `maps.Insert(m, iter)` |
| `reflect.DeepEqual` for maps | `maps.Equal(a, b)` |

---

## 5. Strings & Bytes

| Avoid | Prefer |
|---|---|
| `strings.Index(s, sep) >= 0` with slicing | `strings.Cut(s, sep)` (Go 1.18) |
| `strings.Split(s, sep)` then `range` | `strings.SplitSeq(s, sep)` (Go 1.24) |
| `[]byte(fmt.Sprintf(...))` | `fmt.Appendf(nil, ...)` (Go 1.19) |
| `strings.Builder` + manual `fmt.Sprintf` | `fmt.Fprintf(&sb, ...)` |
| Repeated `s += ...` in a loop | `strings.Builder` |
| `bytes.IndexByte` + slice arithmetic | `bytes.Cut` (Go 1.18) |

---

## 6. For Loops

| Avoid | Prefer |
|---|---|
| `for i := 0; i < n; i++ { ... }` | `for range n { ... }` (Go 1.22) |
| `for i := 0; i < n; i++ { use(i) }` | `for i := range n { use(i) }` (Go 1.22) |
| `for _, x := range s { x := x ... }` (loop-var capture) | Remove the `x := x` re-declaration (Go 1.22 loop semantics) |

---

## 7. Iterators (Go 1.23+)

Use `iter.Seq[V]` and `iter.Seq2[K, V]` for custom collection iteration. Prefer `range` over function iterators when the collection implements `All() iter.Seq[V]`. Use `slices.All`, `slices.Values`, `maps.All`, `maps.Keys`, `maps.Values` where applicable.

---

## 8. Error Handling

- Always wrap errors with context: `fmt.Errorf("context: %w", err)`.
- Use `errors.Is` / `errors.As` for unwrapping, never string comparison.
- Define sentinel errors as `var ErrFoo = errors.New("foo")` at package level.
- Use `errors.Join` (Go 1.20) to combine multiple errors instead of manual concatenation.
- Never silently discard errors with `_` unless the function is explicitly documented as infallible.

---

## 9. Structs & Interfaces

- **Accept interfaces, return structs.** Function parameters should be interface types (for testability); return values should be concrete types (for usability).
- **Declare interfaces at the consumer, not the producer.** Define an interface in the package that *uses* it, not the package that implements it. Keep interfaces small (1–3 methods).
- **Never embed a concrete struct in a public API to satisfy an interface.** Use composition explicitly.
- Use struct field tags consistently: `json:"field_name,omitempty"` or `json:"field_name,omitzero"` (Go 1.24 for `omitzero`).
- Use named struct initialization (`Foo{Field: val}`) always; never positional initialization.

---

## 10. Concurrency

- Use `sync.WaitGroup.Go(func())` (Go 1.25) instead of `wg.Add(1); go func() { defer wg.Done(); ... }()`.
- Prefer `golang.org/x/sync/errgroup` for goroutine fan-out with error collection.
- Use `context.Context` as the first parameter in every function that does I/O or launches goroutines.
- Never use `time.Sleep` for synchronization; use channels or `sync` primitives.
- Protect shared state with `sync.Mutex` or `sync.RWMutex`; prefer `sync/atomic` for simple counters.

---

## 11. Context

- Pass `context.Context` as the **first parameter**, named `ctx`, in any function performing I/O, network calls, or waiting.
- Derive child contexts with deadlines/cancellations; always call the cancel function (`defer cancel()`).
- Never store a `context.Context` in a struct field unless absolutely unavoidable (e.g., HTTP middleware).

---

## 12. Testing

- Use `t.Context()` (Go 1.24) to obtain a test-scoped context instead of `context.WithCancel`.
- Use `testing/synctest` (Go 1.25) for testing concurrent or time-dependent code.
- Use table-driven tests with `t.Run`.
- Use `t.Helper()` in helper functions so failures point to the call site.
- Use `t.Cleanup` for teardown; never rely on `defer` alone for test cleanup.
- Use `testdata/` directories for golden files; use `go test -update` patterns.

---

## 13. JSON (encoding/json)

- Use `omitzero` tag (Go 1.24) instead of `omitempty` when you specifically want to omit zero-value structs/numbers.
- Prefer `encoding/json/v2` (Go 1.25 experimental) for new code in packages that opt in.
- Validate JSON tags with `go vet` (`structtag` checker).

---

## 14. Package & Project Structure

- Flat package structure is preferred over deep nesting.
- Avoid `internal/` unless the package truly must not be imported externally.
- `main` packages live in `cmd/<name>/main.go`.
- Do not create `util`, `common`, `helpers`, or `misc` packages — put code where it belongs.
- Avoid global variables; inject dependencies via constructors.

---

## 15. Formatting & Toolchain

- All code is `gofmt`-formatted — no exceptions.
- Run `go vet ./...` and treat every warning as an error.
- Run `go fix ./...` (Go 1.26+) or `go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest ./...` before finalizing code.
- Use `go tool fix help` to list available auto-fixers and apply them.

---

## 16. Generics (Go 1.18+)

- Use generics when a function operates identically on multiple types that share a constraint — do not duplicate code.
- Prefer existing generic functions from `slices`, `maps`, `cmp` packages over rolling your own.
- Use `cmp.Compare` and `cmp.Ordered` for ordered type constraints.
- Do not over-generify; if a function is only ever called with one concrete type, keep it concrete.

---

## 17. Logging

- Use `log/slog` (Go 1.21) as the default structured logger; never use `log.Printf` in library code.
- Use `slog.GroupAttrs` (Go 1.25) to batch attributes.
- Accept `*slog.Logger` as a dependency; never call `slog.Default()` inside libraries.

---

## 18. Reflection

- Prefer `reflect.TypeAssert[T](v)` (Go 1.25) over type assertions with intermediate allocations where applicable.
- Minimize use of `reflect` in hot paths; use generics instead where possible.

---

## Quick Reference: "Modernize" Checklist

Before submitting any Go code, verify:

- [ ] No `interface{}` — replaced with `any`
- [ ] No manual min/max if-else — use built-in `min`/`max`
- [ ] No `for i := 0; i < n; i++` — use `for range n`
- [ ] No `x := x` loop-var re-declaration (Go 1.22+ semantics)
- [ ] No `strings.Index` + slice — use `strings.Cut`
- [ ] No `[]byte(fmt.Sprintf(...))` — use `fmt.Appendf`
- [ ] No `append([]T(nil), s...)` — use `slices.Clone`
- [ ] No `sort.Slice` for comparable types — use `slices.Sort`
- [ ] No manual map copy loops — use `maps.Copy`/`maps.Clone`
- [ ] No `errors.New(fmt.Sprintf(...))` — use `fmt.Errorf("%w", ...)`
- [ ] No `sync.WaitGroup` boilerplate for goroutines — use `wg.Go(f)` (Go 1.25)
- [ ] No `context.WithCancel` in tests — use `t.Context()`
- [ ] `omitzero` vs `omitempty` considered for JSON struct tags
- [ ] `log/slog` used for structured logging (not `fmt.Println` or `log.Printf`)
- [ ] All errors wrapped with `%w` and checked with `errors.Is`/`errors.As`
