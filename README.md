# ecosystem::parser

Composable text and binary parsers implemented in GoML. Parsers retain concrete
function values, allowing generic transformations and recursive grammars across
package and module boundaries without a runtime type-erasure convention.

```goml
use ecosystem::parser;

fn numbers(input: string) -> Result[Vec[isize], parser::Error] {
    parser::token(parser::integer())
        .separated(parser::token(parser::literal(",")), 0, false)
        .between(parser::literal("["), parser::literal("]"))
        .parse(input)
}
```

## Text API

`Parser[T]::new` accepts `(string, isize) -> Result[(T, isize), Error]`.
`parse` requires complete consumption; `parse_at` accepts a byte offset and
returns the next offset; `parse_prefix` returns the remaining text. The parser
checks offsets, forward progress and UTF-8 boundaries before exposing a result.

### Resource budgets

Every text and binary entry point starts with `ParseLimits::standard()`: 256
nested parser calls and 1,000,000 work units. `parse_with_limits(input, limits)`
overrides these limits; depths must be 1 through 4096 and work nonnegative.
One `ParseContext` is shared by all built-in combinators, alternatives, repetitions,
dependent parsers and `lazy` calls in that parse. Backtracking never replenishes
work. Each invocation costs one unit; byte scans, literal comparisons, character-set
comparisons and error expectation concatenation also consume work. `one_of` and
`none_of` charge one unit per inspected set scalar and stop at the first match;
large sets can therefore exhaust limits that previously counted only invocation.
Binary `take` returns a slice in constant time and charges its invocation rather
than every returned byte.

Depth/work exhaustion returns a committed error at the stopping byte offset.
It remains sticky through `attempt`, `label`, `optional`, negative lookahead and
user callbacks that swallow the immediate error. Left-recursive grammars
therefore fail recoverably at the depth limit instead of overflowing the stack;
they still need rewriting to parse successfully. The iterative `choice` helper
does not add one call-stack frame per alternative.

For custom combinators, use `Parser::with_context` with a
`(ParseContext, input, offset)` callback and call children with
`child.parse_in(context, input, offset)`. `ParseContext::new`, `charge`,
`work_used`, `peak_depth` and `limit_error` support explicit work accounting and
diagnostics. Context copies share their work budget and failure state; use a
fresh context for an independent parse. `nested` is the invocation-accounting
hook used by parser implementations, and callers ordinarily let `parse_in` use it.

The original two-argument `Parser::new` constructor remains available for leaf
callbacks. Work inside arbitrary callbacks cannot be preempted or measured;
calling a top-level `parse`/`parse_at` from one starts an independent budget.
Custom nested parsers must use the contextual constructor to participate in the
outer limit. These budgets bound library work, not wall time or exact heap size.

| Category | API |
| --- | --- |
| Transformation | `map`, `try_map`, `and_then`, `verify` |
| Sequencing | `then`, `left`, `right`, `between` |
| Alternatives | `or`, `choice`, `optional`, `pure`, `fail`, `lazy` |
| Repetition | `many`, `many1`, `many_until(terminator)`, `repeat(minimum, maximum)`, `separated(separator, minimum, trailing)` |
| Observation | `peek`, `not`, `position`, `end`, `spanned`, `recognize` |
| Errors | `cut`, `attempt`, `label`, `context`, `Error::render` |
| Recovery | `recover`, `recover_many`, `Recovery`, `Report[T]`, `collect_recovered` |
| Operators | `chain_left`, `chain_right` |
| Text primitives | `literal`, `take_while`, `take_until`, `satisfy`, `character`, `any_char`, `one_of`, `none_of` |
| Lexical helpers | `whitespace`, `token`, `digits`, `integer`, `decimal`, `identifier`, `quoted`, `line_ending`, `rest_of_line` |

`or` backtracks unless the failed branch is committed with `cut`. It reports the
furthest error and combines expectations on a tie. `attempt` clears commitment.
`between` commits after its opening delimiter. Repetitions reject successful
zero-width iterations instead of looping forever. Lists explicitly choose
whether a trailing separator is accepted. `item.many_until(terminator)` returns `(items, ending)`, checking the terminator
before every item and consuming it on success. This parses sentinel-delimited
text even when the item could consume the sentinel's prefix. Use a peeked
terminator to retain it, or `end()` to require EOF. Zero items and zero-width
terminators are allowed; successful items must advance. A committed error from
either parser propagates; otherwise a failed item reports the furthest item or
terminator error and combines tied expectations. Both parsers share the original
work/depth budget, including failed terminator probes. Missing terminators fail
rather than returning a partial list.

`peek` retains input; `not` preserves
committed errors. Error offsets and spans use bytes; rendered columns count
Unicode scalar values, not terminal display cells or UTF-16 units.

`identifier` accepts Unicode letters and underscore initially, then Unicode
letters/numbers and underscore. `integer` accepts an optional sign and decimal
digits with machine-range checks. `decimal` additionally accepts a fractional
part and exponent; a decimal point or exponent marker requires following digits.
`quoted` supports the selected quote and `\\`, `\n`, `\r`, `\t` escapes.
`take_while` uses byte predicates and rejects an ending inside a UTF-8 scalar.
`take_until(marker)` returns the text before the first literal marker and retains
the marker for a following parser. Empty markers are invalid; absence fails at
EOF. An immediate marker produces an empty string. A prefix table avoids repeated
rescanning of overlapping delimiters: work is O(input scanned + marker bytes),
with O(marker bytes) scratch space. Table allocation and comparisons share the
parse work budget, including failures; limits remain sticky through backtracking.
Use `.left(literal(marker))` to consume the delimiter as well. The byte search
preserves UTF-8 boundaries and allocates no vector of parsed characters.

`arithmetic::expression()` demonstrates recursive parentheses, multiplication,
addition and subtraction with precedence. `lazy` supports productive recursive
grammars; left recursion must be eliminated. Text parsers consume an available
string, while incremental binary parsing uses the API below.

### Explicit error recovery

Strict parsers and `parse` keep their existing behavior. Opt into recovery at a
grammar boundary whose malformed input can be discarded:

```goml
use ecosystem::parser;

fn configuration(input: string) -> Result[parser::Report[Vec[(string, isize)]], parser::Error] {
    let record = parser::token(parser::identifier())
        .left(parser::literal("="))
        .then(parser::token(parser::integer()))
        .left(parser::token(parser::literal(";")))
        .cut();
    let recovery = parser::Recovery::until(Vec::from_array([";"]))
        .nested("[", "]")
        .nested("(", ")")
        .quoted('"');
    parser::whitespace().right(record.recover_many(recovery)).parse(input)
}
```

For `port=8080;broken;workers=4;`, the report contains both valid assignments in
`value` and one original parser error in `errors`. Recoverable syntax errors,
including committed errors, become report data. A `Result::Err` still indicates
failure of the surrounding grammar, invalid configuration, lack of repetition
progress, or exhausted resource limits. Always inspect `errors` before treating
the input as valid; recovery never invents a replacement value.

`recover(strategy)` produces `Parser[Report[Option[T]]]`: success supplies
`Some(value)` and no errors; recovery supplies `None` and the original error.
`recover_many(strategy)` repeatedly parses complete records until EOF, returning
`Report[Vec[T]]` with successful values and errors in source order. Empty input
produces an empty report without attempting a record. The example above and its
tests also run through `goml verify` as an independent downstream module.

`Recovery::until(markers)` consumes the first top-level synchronization marker.
`before()` retains that marker for a surrounding separator or closing-delimiter
parser. An empty marker list means skip to EOF; empty marker strings are invalid.
`nested(open, close)` protects balanced delimiter pairs, and `quoted(quote)`
protects quoted text with backslash escaping of the following Unicode scalar.
No delimiter, quote, escape beyond backslash, or comment convention is implicit.
Delimiter strings must be nonempty and distinct within each pair. Overlapping
markers and opening delimiters select the longest match; equal opening strings
use the first configured pair. At top level, a synchronization marker takes
precedence over a quote or opening delimiter; inside a nested region the current
closing delimiter takes precedence. Builders snapshot their collections.

Scanning restarts at the failed parser's input offset, including already consumed
opening delimiters. Markers inside protected regions are ignored. Mismatched
closing delimiters are skipped as ordinary text; an unclosed region or quote
conservatively consumes through EOF and reports the original error once. Offsets
remain UTF-8 byte offsets. Scans, marker comparisons, delimiter validation, and
protected nesting share the parse's work/depth limits. Comparing equal-length
opening/closing delimiters charges their byte length before inspecting contents.
Delimiters of different lengths require only a constant-time length check. This
validation also runs before a successful child parse. Exhaustion remains sticky
and cannot become a successful recovery report, even through `attempt`,
`optional`, or callbacks.

Use `collect_recovered` to combine individually recovered items into a list report:

```goml
let boundary = parser::literal(",").or(parser::literal("]")).peek();
let item = parser::integer().left(boundary).recover(
    parser::Recovery::until(Vec::from_array([",", "]"])).before().nested("(", ")"),
);
let list = item.separated(parser::literal(","), 1, false)
    .map(parser::collect_recovered)
    .between(parser::literal("["), parser::literal("]"));
```

`[1,bad(2,3),,5]` retains `1` and `5` and reports two bad items. Here the list
separator guarantees progress even for an empty bad item. `recover` itself may
return without advancing at an immediate retained marker or EOF; ordinary `many`
and `recover_many` reject such iterations. Use `recover_many` for whole records
whose successful and recovered forms both advance. Reports travel as parser
values, so a failed alternative cannot leave diagnostics in shared context.
Binary incomplete-frame handling is unchanged; this recovery API applies to text.

## Binary API

Import `ecosystem::parser::binary`. Its separate `Parser[T]` takes a
`Slice[byte]` and supports `parse`, `parse_at`, `map`, `try_map`, `then`, `left`,
`right`, `and_then`, bounded `repeat`, `many_until`, `or`, `cut`, `attempt`, `optional`, `peek`,
`not`, `verify` and `between`. `pure`, `lazy` and `end` support recursive layouts.
The contextual constructor and resource-limit APIs match the text parser.

`take`, `literal`, `byte_value`, `uint16/32/64`, `int16/32/64`, `float32/64` and
`length_prefixed` cover common frame layouts. Numeric parsers take
`std::bytes::endian::Endian`. Slices retain the input backing storage, and literal
patterns are snapshotted at construction. Do not concurrently mutate input views.

`Error::Incomplete { offset, needed }` distinguishes a valid truncated prefix
from `Error::Invalid`. Append more input and retry `parse_at` at the frame start;
the parsers keep no hidden partial state. `length_prefixed` validates its maximum
before waiting for or slicing the payload. The caller owns buffering and I/O.
Binary alternatives backtrack only on uncommitted invalid input. `Incomplete`
propagates unchanged through alternatives, optional parsing and lookahead so a
short transport read cannot prematurely select a different branch. `cut` and
`attempt` affect invalid errors, preserving the needed-byte count on truncation.

`item.many_until(terminator)` returns `(items, ending)` and tries the terminator
before each item, consuming it on success. A peeked terminator retains its bytes;
`end()` terminates at EOF. Zero items and zero-width terminators are accepted, but
every successful item must advance. An incomplete terminator prefix propagates
unchanged without trying the item, so a fragmented sentinel is not consumed as
payload. Incomplete items and committed invalid errors also propagate unchanged.
Other failures retain the furthest diagnostic and combine tied expectations.
Terminator probes, item parsing and diagnostics share the outer depth/work budget.

## Text scanner

Import `ecosystem::parser::scanner` for an independent pull scanner over a UTF-8
string. `Scanner::new(input, max_token_bytes)` requires a positive byte limit and
returns identifier, decimal-digit, quoted and single-scalar punctuation tokens.
Identifiers use Unicode letters and numbers with `_`; decimal digits form number
tokens. Single- and double-quoted tokens retain their original spelling, including
escapes. The scanner checks for a closing quote and rejects a newline or truncated
escape, leaving language-specific escape interpretation to the caller.

Each token contains its source slice and start/end byte offset, one-based line and
one-based scalar column. LF advances the line; CR is a separate whitespace scalar.
`Options` controls whitespace emission and optional `//` and `/* */` comments.
Comments are skipped by default when enabled, or returned intact. Skipped runs
still obey the token byte limit. Invalid options, excessive tokens and unterminated
quotes/comments return recoverable errors. Failure is sticky; repeated calls do
not scan further. Scanner copies share cursor and failure state and require
serialized access. EOF returns `Ok(None)` repeatedly.

This scanner is a small reusable lexical layer. Numeric grammar, comment policy
and literal decoding belong to each consuming language or format; Go-specific
token rules remain in gomlgo.

## Validation

```sh
(cd ../verification && just ecosystem-test parser)
```

The module tests cover precedence, commitment, backtracking, invalid ranges,
zero-width repetitions, Unicode spans, errors, malformed numbers and every prefix
of a binary frame. The example uses the `ecosystem::proptest` development dependency to check
integer and list roundtrips. `goml verify` repeats these checks against an independent registry snapshot.
Budget tests cover left recursion, custom nested callbacks, shared sibling work,
repetition, error relabeling, swallowed errors and binary prefix alternatives.
Recovery tests cover multiple malformed records, list composition, nested and
quoted markers, consumed openers, UTF-8 offsets, EOF, zero progress, resource
exhaustion, configuration snapshots, and backtracked diagnostics.

## Development and examples

Requires GoML 0.1.56 or newer. The `examples/basic/` example shares the root manifest; test-only helpers are declared in `[dev-dependencies]`. From the library root, run:

```sh
goml run --example basic
goml test
goml verify --timeout 300s
```

`goml test` builds the example and runs its tests. `goml verify` repeats the example checks as an independent module against an isolated registry snapshot. `(cd ../verification && just ecosystem-test parser)` also retains the library-specific smoke and compatibility checks.
