# 1. Core Principle

Write code that is easy to read, debug, and change.

Prefer:

- lower complexity over fewer lines
- explicit behavior over clever abstraction
- local clarity over theoretical reuse
- concrete code over framework-like indirection

The goal is not to remove all duplication. The goal is to reduce unnecessary complexity.

---

# 2. Optimize for Readability, Not Cleverness

Do not compress code just to make it shorter.

Avoid:

- clever one-liners that hide control flow
- abstractions introduced only to look elegant
- generic utilities that make simple logic harder to follow
- “code golf” style implementations

Prefer code that makes the real behavior obvious on first read.

---

# 3. Duplicate When the Differences Matter

Do not force different logic into one abstraction just because the structure looks similar.

Keep code separate when:

- the business rules are meaningfully different
- platform-specific behavior differs
- error handling differs in important ways
- future changes are likely to diverge

In Go, this means it is often better to keep two explicit functions than to create one generalized helper with many flags, callbacks, or mode parameters.

Good rule:

- duplicate when the logic is only superficially similar
- abstract only when the shared behavior is truly the same

---

# 4. Extract Helpers Only When They Express a Real Concept

Do not split code into helpers just to shorten a function.

Create a helper when:

- the logic has a clear name
- the logic is reused
- the helper makes the caller easier to understand
- the extracted code represents a domain concept or transformation

Bad reasons to extract:

- “this function is getting long”
- “I want fewer repeated lines”
- “it feels more modular”

A helper should clarify the code, not scatter it.

---

# 5. Keep One-Off Logic Local

If a piece of logic is only relevant inside one function, keep it close to that function.

In Go, prefer:

- a small local closure for tightly scoped logic, when it improves readability
- an inline block if extraction would force the reader to jump around
- a private helper only if the logic is reusable or conceptually important

Do not create package-level helpers for single-use implementation details.

---

# 6. Allow Large Functions When the Flow Is Naturally Unified

Do not split a function just to satisfy an arbitrary size preference.

A larger function is acceptable when it represents:

- one complete workflow
- one tightly coupled algorithm
- one stateful operation
- one dispatch path with meaningful branches

Split a function only when the result improves comprehension.

In Go, large functions are acceptable if:

- the control flow is still readable
- the variables are still understandable
- the extracted pieces have real names and responsibilities

Do not turn one understandable function into six thin wrappers.

---

# 7. Prefer Explicit Branching Over Indirection

When cases are materially different, write the branches explicitly.

In Go, prefer:

- `switch`
- clear `if/else`
- explicit per-case handling

Do not hide important differences behind:

- generic configuration maps
- interface-heavy dispatch
- reflection
- unnecessary callback layers

If readers need to understand how cases differ, let them see the differences directly in code.

---

# 8. Prefer Concrete Types Over Premature Interfaces

Do not start with interfaces unless there is a real consumer that benefits from one.

In Go:

- start with concrete structs and functions
- introduce interfaces at the point of use
- keep interfaces small and behavior-focused
- avoid broad “service” interfaces that exist only for architecture aesthetics

Use interfaces when they help with:

- isolating a true dependency boundary
- testing meaningful behavior
- supporting real multiple implementations

Do not use interfaces to simulate flexibility you do not need yet.

---

# 9. Use Data-Driven Rules Only When the Domain Truly Fits

Some problems are naturally expressed as rules, tables, or pattern mappings. Use those forms when they make the logic clearer than branching.

In Go, this can mean:

- lookup tables
- rule slices
- small strategy maps
- explicit transformation pipelines

But do not force a data-driven design when a plain `switch` is easier to read.

Use declarative structure when it reflects the problem. Do not use it just to feel abstract.

---

# 10. Locality Matters

Keep related logic close together.

Prefer:

- helper functions in the same file as their caller when they are part of the same concept
- structs and methods grouped by responsibility
- short call chains
- code that can be understood without excessive jumping across files

Avoid spreading one idea across too many files, types, or helpers.

A reader should be able to follow the main path with minimal navigation.

---

# 11. Accept Some Repetition to Preserve Clarity

A little duplication is often cheaper than the wrong abstraction.

In Go, this often means:

- repeating a few lines in two handlers
- keeping separate encode/decode paths
- writing explicit validation per input type
- keeping backend-specific or transport-specific logic distinct

Do not build shared utilities too early.

Use the rule of practical repetition:

- first occurrence: write it simply
- second occurrence: notice it
- third occurrence: consider abstraction
- only extract if the abstraction is clearly simpler than the duplication

---

# 12. Avoid Abstractions That Need Flags to Behave Correctly

Be suspicious of helpers like:

- `process(x, true, false)`
- `run(mode, opts...)`
- `build(kind, withCache, withRetry, skipValidation)`

These often indicate unrelated behaviors being forced into one function.

In Go, if a helper needs many booleans or many mode switches, it is often better to split it into separate functions.

Prefer code with names that explain behavior over parameters that toggle behavior.

---

# 13. Make Important Differences Obvious in API Design

When designing Go APIs:

- use names that reflect the actual action
- make distinct behaviors separate functions or methods
- avoid “do everything” entry points
- keep signatures small and intention-revealing

Examples:

- prefer `LoadFromFile` and `LoadFromReader` over one function with mode branching
- prefer `MarshalJSON` and `MarshalBinary` style separation over format flags
- prefer `CreateUser` and `CreateAdminUser` if the workflows differ meaningfully

---

# 14. Refactor Only for Clear Wins

A refactor is worthwhile only if it clearly improves:

- readability
- maintainability
- conceptual simplicity
- correctness confidence

A refactor is not justified if it only:

- moves code around
- reduces duplication at the cost of clarity
- introduces extra layers
- makes code look more “architected”

Before refactoring, ask:

- Is the repeated logic actually the same?
- Will this make future changes easier?
- Will the code be easier to understand for someone new?
- Am I reducing complexity, or just moving it?

---

# 15. Testing Should Follow Real Structure

Tests should validate behavior, not abstractions for their own sake.

In Go:

- test the real public behavior
- prefer table-driven tests when they clarify case coverage
- do not introduce abstraction in production code only to make tests prettier
- do not invent interfaces solely to mock trivial dependencies

If a concrete dependency is simple, use it directly. If a boundary is real, abstract it deliberately.

---

# 16. Practical Go Heuristics

Use these as defaults:

## Functions
- Keep functions focused.
- Allow larger functions if they represent one coherent flow.
- Extract helpers only when they have a strong name and purpose.

## Parameters
- Avoid too many parameters.
- If parameters naturally belong together, use a struct.
- Do not create config structs for one small internal function unless they improve clarity.

## Interfaces
- Define interfaces at the consumer side.
- Keep them small.
- Prefer concrete types until polymorphism is genuinely needed.

## Packages
- Organize packages by responsibility, not by abstract layers.
- Avoid generic `util`, `common`, `base`, or `shared` dumping grounds.
- Keep related concepts together.

## Error handling
- Handle errors explicitly.
- Do not hide meaningful error differences behind generic wrappers unless the caller truly benefits.

## Concurrency
- Keep goroutine coordination visible.
- Prefer explicit channels, contexts, and synchronization over elaborate concurrency abstractions.

---

# 17. Summary

The tinygrad-inspired Go style can be summarized as:

- write direct code first
- keep important differences explicit
- tolerate some duplication
- abstract only when it clearly reduces complexity
- use helpers to name concepts, not to chase shorter functions
- prefer concrete types over premature interfaces
- keep logic local and navigable
- value readability over elegance

A good Go abstraction should make the code easier to understand.
If it only makes the code look more sophisticated, it is probably the wrong abstraction.
