<!-- file: .github/instructions/r.instructions.md -->
<!-- version: 1.3.0 -->
<!-- guid: 6c5b4a3c-2d1e-0f9a-8b7c-6d5e4f3a2b1c -->
<!-- DO NOT EDIT: This file is managed centrally in ghcommon repository -->
<!-- To update: Create an issue/PR in jdfalk/ghcommon -->

<!-- prettier-ignore-start -->
<!-- markdownlint-disable -->
---
applyTo: "**/*.R"
description: |
  Coding, documentation, and workflow rules for R files, following Google/Tidyverse R style guide and general project rules. Reference this for all R code, documentation, and formatting in this repository. All unique content from the Google/Tidyverse R Style Guide is merged here.
---
<!-- markdownlint-enable -->
<!-- prettier-ignore-end -->

# R Coding Instructions

- Follow the [general coding instructions](general-coding.instructions.md).
- Follow the
  [Google R Style Guide](https://google.github.io/styleguide/Rguide.html) and
  [Tidyverse Style Guide](https://style.tidyverse.org/) for additional best
  practices.
- All R files must begin with the required file header (see general instructions
  for details and R example).

## Naming Conventions

- Use snake_case for variables and functions
- Variable names should be nouns, function names should be verbs
- Google prefers BigCamelCase for functions, Tidyverse prefers snake_case
- Private functions should start with a dot
- Avoid problematic names and reserved words

## Syntax and Formatting

- Always space after commas, never before
- No spaces inside parentheses for function calls
- Most infix operators surrounded by spaces
- Limit lines to 80 characters
- Break long function calls with one argument per line
- Use 2 spaces for indentation
- Use braced expressions for control flow and loops
- Use `&&` and `||` in conditions
- Use descriptive names, avoid single character names

## Required File Header

All R files must begin with a standard header as described in the
[general coding instructions](general-coding.instructions.md). Example for R:

```r
# file: path/to/file.R
# version: 1.0.0
# guid: 123e4567-e89b-12d3-a456-426614174000
```

For executable R scripts (Rscript), include shebang BEFORE the file header:

```r
#!/usr/bin/env Rscript
# file: path/to/script.R
# version: 1.0.0
# guid: 123e4567-e89b-12d3-a456-426614174000
```

---

## Google R Style Guide (Complete)

*Note: The Google R Style Guide is a fork of the Tidyverse Style Guide with specific Google modifications. Both are included here for comprehensive coverage.*

### Syntax

#### Naming Conventions

Google prefers identifying functions with `BigCamelCase` to clearly distinguish them from other objects.

```r
# Good
DoNothing <- function() {
  return(invisible(NULL))
}
```

The names of private functions should begin with a dot. This helps communicate both the origin of the function and its intended use.

```r
# Good
.DoNothingPrivately <- function() {
  return(invisible(NULL))
}
```

Google previously recommended naming objects with `dot.case`. They're moving away from that, as it creates confusion with S3 methods.

#### Don't use attach()

The possibilities for creating errors when using `attach()` are numerous.

### Pipes

#### Right-hand assignment

Google does not support using right-hand assignment.

```r
# Bad
iris %>%
  dplyr::summarize(max_petal = max(Petal.Width)) -> results
```

This convention differs substantially from practices in other languages and makes it harder to see in code where an object is defined. E.g. searching for `foo <-` is easier than searching for `foo <-` and `-> foo` (possibly split over lines).

#### Use explicit returns

Do not rely on R's implicit return feature. It is better to be clear about your intent to `return()` an object.

```r
# Good
AddValues <- function(x, y) {
  return(x + y)
}

# Bad
AddValues <- function(x, y) {
  x + y
}
```

#### Qualifying namespaces

Users should explicitly qualify namespaces for all external functions.

```r
# Good
purrr::map()
```

Google discourages using the `@import` Roxygen tag to bring in all functions into a NAMESPACE. Google has a very big R codebase, and importing all functions creates too much risk for name collisions.

While there is a small performance penalty for using `::`, it makes it easier to understand dependencies in your code. There are some exceptions to this rule:

- Infix functions (`%name%`) always need to be imported.
- Certain `rlang` pronouns, notably `.data`, need to be imported.
- Functions from default R packages, including `datasets`, `utils`, `grDevices`, `graphics`, `stats` and `methods`. If needed, you can `@import` the full package.

When importing functions, place the `@importFrom` tag in the Roxygen header above the function where the external dependency is used.

### Documentation

#### Package-level documentation

All packages should have a package documentation file, in a `packagename-package.R` file.

---

## Tidyverse Style Guide (Complete)

### Files

#### Names

1. **File names should be machine readable**: avoid spaces, symbols, and special characters. Prefer file names that are all lower case, and never have names that differ only in their capitalization. Delimit words with `-` or `_`. Use `.R` as the extension of R files.

```r
# Good
fit_models.R
utility_functions.R
exploratory-data-analysis.R

# Bad
fit models.R
foo.r
ExploratoryDataAnaylsis.r
```

2. **File names should be human readable**: use file names to describe what's in the file.

```r
# good
report-draft-notes.txt

# bad
temp.r
```

Use the same structure for closely related files:

```r
# good
fig-eda.png
fig-model-3.png

# bad
figure eda.PNG
fig model three.png
```

3. **File names should play well with default ordering**. If your file names contain dates, use yyyy-mm-dd (ISO8601) format so they sort in chronological order. If your file names include numbers, make sure to pad them with the appropriate number of zeros so that (e.g.) 11 doesn't get sorted before 2. If files should be used in a specific order, put the number at the start, not the end.

```r
# good
01-load-data.R
02-exploratory-analysis.R
03-model-approach-1.R
04-model-approach-2.R
2025-01-01-report.Rmd
2025-02-01.report.Rmd

# bad
alternative model.R
code for exploratory analysis.r
feb 01 report.Rmd
jan 01 report.Rmd
model_first_try.R
run-first.r
```

If you later realise that you've missed some steps, it's tempting to use `02a`, `02b`, etc. However, it's generally better to bite the bullet and rename all files.

4. **Don't tempt fate** by using "final" or similar words in file names. Instead either rely on Git to track changes over time, or failing that, put the date in the file name.

```r
# good
report-2022-03-20.qmd
report-2022-04-02.qmd

# bad
finalreport.qmd
FinalReport-2.qmd
```

#### Organisation

It's hard to describe exactly how you should organise your code across multiple files. The best rule of thumb is that if you can give a file a concise name that still evokes its contents, you've arrived at a good organisation.

#### Internal structure

Use commented lines of `-` and `=` to break up your file into easily readable chunks.

```r
# Load data ---------------------------

# Plot data ---------------------------
```

If your script uses add-on packages, load them all at once at the very beginning of the file. This is more transparent than sprinkling `library()` calls throughout your code or having hidden dependencies that are loaded in a startup file, such as `.Rprofile`.

### Syntax

#### Object names

> "There are only two hard things in Computer Science: cache invalidation and naming things." — Phil Karlton

Variable and function names should use only lowercase letters, numbers, and `_`. Use underscores (`_`) (so called snake case) to separate words within a name.

```r
# Good
day_one
day_1

# Bad
DayOne
dayone
```

Base R uses dots in function names (`contrib.url()`) and class names (`data.frame`), but it's better to reserve dots exclusively for the S3 object system. In S3, methods are given the name `function.class`; if you also use `.` in function and class names, you end up with confusing methods like `as.data.frame.data.frame()`.

If you find yourself attempting to cram data into variable names (e.g. `model_2018`, `model_2019`, `model_2020`), consider using a list or data frame instead.

Generally, variable names should be nouns and function names should be verbs. Strive for names that are concise and meaningful (this is not easy!).

```r
# Good
day_one

# Bad
first_day_of_the_month
djm1
```

Where possible, avoid re-using names of common functions and variables. This will cause confusion for the readers of your code.

```r
# Bad
T <- FALSE
c <- 10
mean <- function(x) sum(x)
```

#### Spacing

##### Commas

Always put a space after a comma, never before, just like in regular English.

```r
# Good
x[, 1]

# Bad
x[,1]
x[ ,1]
x[ , 1]
```

##### Parentheses

Do not put spaces inside or outside parentheses for regular function calls.

```r
# Good
mean(x, na.rm = TRUE)

# Bad
mean (x, na.rm = TRUE)
mean( x, na.rm = TRUE )
```

Place a space before and after `()` when used with `if`, `for`, or `while`.

```r
# Good
if (debug) {
  show(x)
}

# Bad
if(debug){
  show(x)
}
```

Place a space after `()` used for function arguments:

```r
# Good
function(x) {}

# Bad
function (x) {}
function(x){}
```

##### Embracing

The embracing operator, `{ }`, should always have inner spaces to help emphasise its special behaviour:

```r
# Good
max_by <- function(data, var, by) {
  data |>
    group_by({{ by }}) |>
    summarise(maximum = max({{ var }}, na.rm = TRUE))
}

# Bad
max_by <- function(data, var, by) {
  data |>
    group_by({{by}}) |>
    summarise(maximum = max({{var}}, na.rm = TRUE))
}
```

##### Infix operators

Most infix operators (`==`, `+`, `-`, `<-`, etc.) should always be surrounded by spaces:

```r
# Good
height <- (feet * 12) + inches
mean(x, na.rm = TRUE)

# Bad
height<-feet*12+inches
mean(x, na.rm=TRUE)
```

There are a few exceptions, which should never be surrounded by spaces:

- The operators with high precedence: `::`, `:::`, `$`, `@`, `[`, `[[`, `^`, unary `-`, unary `+`, and `:`.

```r
# Good
sqrt(x^2 + y^2)
df$z
x <- 1:10

# Bad
sqrt(x ^ 2 + y ^ 2)
df $ z
x <- 1 : 10
```

- Single-sided formulas when the right-hand side is a single identifier.

```r
# Good
~foo
tribble(
  ~col1, ~col2,
  "a",   "b"
)

# Bad
~ foo
tribble(
  ~ col1, ~ col2,
  "a", "b"
)
```

Note that single-sided formulas with a complex right-hand side do need a space.

```r
# Good
~ .x + .y

# Bad
~.x + .y
```

- When used in tidy evaluation `!!` (bang-bang) and `!!!` (bang-bang-bang) (because they have precedence equivalent to unary `-`/`+`).

```r
# Good
call(!!xyz)

# Bad
call(!! xyz)
call( !! xyz)
call(! !xyz)
```

- The help operator.

```r
# Good
package?stats
?mean

# Bad
package ? stats
? mean
```

##### Extra spaces

Adding extra spaces is ok if it improves alignment of `=` or `<-`.

```r
# Good
list(
  total = a + b + c,
  mean  = (a + b + c) / n
)

# Also fine
list(
  total = a + b + c,
  mean = (a + b + c) / n
)
```

Do not add extra spaces to places where space is not usually allowed.

#### Vertical space

Use vertical whitespace sparingly, and primarily to separate your "thoughts" in code, much like paragraph breaks in prose.

- Avoid empty lines at the start or end of functions.
- Only use a single empty line when needed to separate functions or pipes.
- It often makes sense to put an empty line before a comment block, to help visually connect the explanation with the code that it applies to.

#### Function calls

##### Named arguments

A function's arguments typically fall into two broad categories: one supplies the data to compute on; the other controls the details of computation. When you call a function, you typically omit the names of data arguments, because they are used so commonly. If you override the default value of an argument, use the full name:

```r
# Good
mean(1:10, na.rm = TRUE)

# Bad
mean(x = 1:10, , FALSE)
mean(, TRUE, x = c(1:10, NA))
```

Avoid partial matching, where you supply a unique prefix of a function argument.

```r
# Good
rep(1:2, times = 3)
cut(1:10, breaks = c(0, 4, 11))

# Bad
rep(1:2, t = 3)
cut(1:10, br = c(0, 4, 11))
```

##### Assignment

Avoid assignment in function calls:

```r
# Good
x <- complicated_function()
if (nzchar(x) < 1) {
  # do something
}

# Bad
if (nzchar(x <- complicated_function()) < 1) {
  # do something
}
```

The only exception is in functions that capture side-effects:

```r
output <- capture.output(x <- f())
```

##### Long function calls

Strive to limit your code to 80 characters per line. This fits comfortably on a printed page with a reasonably sized font. If you find yourself running out of room, this is a good indication that you should encapsulate some of the work in a separate function or use early returns to reduce the nesting in your code.

If a function call is too long to fit on a single line, use one line each for the function name, each argument, and the closing `)`. This makes the code easier to read and to change later.

```r
# Good
do_something_very_complicated(
  something = "that",
  requires = many,
  arguments = "some of which may be long"
)

# Bad
do_something_very_complicated("that", requires, many, arguments,
                              "some of which may be long"
                              )
```

As described under Named arguments, you can omit the argument names for very common arguments (i.e. for arguments that are used in almost every invocation of the function). If this introduces a large disparity between the line lengths, you may want to supply names anyway:

```r
# Good
my_function(
  x,
  long_argument_name,
  extra_argument_a = 10,
  extra_argument_b = c(1, 43, 390, 210209)
)

# Also good
my_function(
  x = x,
  y = long_argument_name,
  extra_argument_a = 10,
  extra_argument_b = c(1, 43, 390, 210209)
)
```

You may place multiple unnamed arguments on the same line if they are closely related to each other. A common example of this is creating strings with `paste()`. In such cases, it's often beneficial to match one line of code to one line of output.

```r
# Good
paste0(
  "Requirement: ", requires, "\n",
  "Result: ", result, "\n"
)

# Bad
paste0(
  "Requirement: ", requires,
  "\n", "Result: ",
  result, "\n")
```

#### Braced expressions

Braced expressions, `{}`, define the most important hierarchy of R code, allowing you to group multiple R expressions together into a single expression. The most common places to use braced expressions are in function definitions, control flow, and in certain function calls (e.g. `tryCatch()` and `test_that()`).

To make this hierarchy easy to see:

- `{` should be the last character on the line. Related code (e.g., an `if` clause, a function declaration, a trailing comma, …) must be on the same line as the opening brace.
- The contents should be indented by two spaces.
- `}` should be the first character on the line.

```r
# Good
if (y < 0 && debug) {
  message("y is negative")
}

if (y == 0) {
  if (x > 0) {
    log(x)
  } else {
    message("x is negative or zero")
  }
} else {
  y^x
}

test_that("call1 returns an ordered factor", {
  expect_s3_class(call1(x, y), c("factor", "ordered"))
})

tryCatch(
  {
    x <- scan()
    cat("Total: ", sum(x), "\n", sep = "")
  },
  interrupt = function(e) {
    message("Aborted by user")
  }
)

# Bad
if (y < 0 && debug) {
message("Y is negative")
}

if (y == 0)
{
    if (x > 0) {
      log(x)
    } else {
  message("x is negative or zero")
    }
} else { y ^ x }
```

It is occasionally useful to have empty braced expressions, in which case it should be written `{}`, with no intervening space.

```r
# Good
function(...) {}

# Bad
function(...) { }
function(...) {

}
```

#### Control flow

##### Loops

R defines three types of looping constructs: `for`, `while`, and `repeat` loops.

- The body of a loop must be a braced expression.

```r
# Good
for (i in seq) {
  x[i] <- x[i] + 1
}

while (waiting_for_something()) {
  cat("Still waiting...")
}

# Bad
for (i in seq) x[i] <- x[i] + 1

while (waiting_for_something()) cat("Still waiting...")
```

- It is occasionally useful to use a `while` loop with an empty braced expression body to wait. As mentioned in Braced expressions, there should be no space within the `{}`.

##### If statements

- A single line if statement must never contain braced expressions. You can use single line if statements for very simple statements that don't have side-effects and don't modify the control flow.

```r
# Good
message <- if (x > 10) "big" else "small"

# Bad
message <- if (x > 10) { "big" } else { "small" }

if (x > 0) message <- "big" else message <- "small"

if (x > 0) return(x)
```

- A multiline if statement must contain braced expressions.

```r
# Good
if (x > 10) {
  x * 2
}

if (x > 10) {
  x * 2
} else {
  x * 3
}

# Bad
if (x > 10)
  x * 2

# In particular, this if statement will only parse when wrapped in a braced
# expression or call
{
  if (x > 10)
    x * 2
  else
    x * 3
}
```

- When present, `else` should be on the same line as `}`.
- Avoid implicit type coercion (e.g. from numeric to logical) in the condition of an if statement:

```r
# Good
if (length(x) > 0) {
  # do something
}

# Bad
if (length(x)) {
  # do something
}
```

`&` and `|` should never be used inside of an `if` clause because they can return vectors. Always use `&&` and `||` instead.

`ifelse(x, a, b)` is not a drop-in replacement for `if (x) a else b`. `ifelse()` is vectorised (i.e. if `length(x) > 1`, then `a` and `b` will be recycled to match) and it is eager (i.e. both `a` and `b` will always be evaluated).

##### Control flow modifiers

Syntax that affects control flow (like `return()`, `stop()`, `break`, or `next`) should always go in their own `{}` block:

```r
# Good
if (y < 0) {
  stop("Y is negative")
}

find_abs <- function(x) {
  if (x > 0) {
    return(x)
  }
  x * -1
}

for (x in xs) {
  if (is_done(x)) {
    break
  }
}

# Bad
if (y < 0) stop("Y is negative")

find_abs <- function(x) {
  if (x > 0) return(x)
  x * -1
}

for (x in xs) {
  if (is_done(x)) break
}
```

##### Switch statements

- Avoid position-based `switch()` statements (i.e. prefer names).
- Each element should go on its own line unless all element can fit on one line.
- Elements that fall through to the following element should have a space after `=`.
- Provide a fall-through error unless you have previously validated the input.

```r
# Good
switch(x,
  a = ,
  b = 1,
  c = 2,
  stop("Unknown `x`", call. = FALSE)
)

# Bad
switch(x,
  a =,
  b = 1,
  c = 2
)
switch(x,
  a = long_function_name1(), b = long_function_name2(),
  c = long_function_name2()
)
switch(y, 1, 2, 3)
```

#### Semicolons

Semicolons are never recommended. In particular, don't put `;` at the end of a line, and don't use `;` to put multiple commands on one line.

```r
# Good
my_helper()
my_other_helper()

# Bad
my_helper();
my_other_helper();

{ my_helper(); my_other_helper() }
```

#### Assignment

Use `<-`, not `=`, for assignment.

```r
# Good
x <- 5

# Bad
x = 5
```

#### Data

##### Character vectors

Use `"`, not `'`, for quoting text. The only exception is when the text already contains double quotes and no single quotes.

```r
# Good
"Text"
'Text with "quotes"'
'<a href="http://style.tidyverse.org">A link</a>'

# Bad
'Text'
'Text with "double" and \'single\' quotes'
```

##### Logical vectors

Prefer `TRUE` and `FALSE` over `T` and `F`.

#### Comments

Each line of a comment should begin with the comment symbol and a single space: `#`

In data analysis code, use comments to record important findings and analysis decisions. If you need comments to explain what your code is doing, consider rewriting your code to be clearer. If you discover that you have more comments than code, consider switching to R Markdown.

### Functions

#### Naming

As well as following the general advice for object names in Section 2.1, strive to use verbs for function names:

```r
# Good
add_row()
permute()

# Bad
row_adder()
permutation()
```

#### Anonymous functions

Use the new lambda syntax: `\(x) x + 1` when writing short anonymous functions (i.e. when you define a function in an argument without giving it an explicit name).

```r
# Good
map(xs, \(x) mean((x + 5)^2))
map(xs, function(x) mean((x + 5)^2))

# Bad
map(xs, ~ mean((.x + 5)^2))
```

Don't use `\()` for multi-line functions:

```r
# Good
map(xs, function(x) {
  mean((x + 5)^2)
})

# Bad
map(xs, \(x) {
  mean((x + 5)^2)
})
```

Or when creating named functions:

```r
# Good
cv <- function(x) {
  sd(x) / mean(x)
}

# Bad
cv <- \(x) sd(x) / mean(x)
```

Avoid using `\()` in a pipe, and remember to use informative argument names.

#### Multi-line function definitions

There are two options if the function name and definition can't fit on a single line. In both cases, each argument goes on its own line; the difference is how deep you indent it and where you put `)` and `{`:

- **Single-indent**: indent the argument name with a single indent (i.e. two spaces). The trailing `)` and leading `{` go on a new line.

```r
# Good
long_function_name <- function(
  a = "a long argument",
  b = "another argument",
  c = "another long argument"
) {
  # As usual code is indented by two spaces.
}
```

- **Hanging-indent**: indent the argument name to match the opening `(` of `function`. The trailing `)` and leading `{` go on the same line as the last argument.

```r
# Good
long_function_name <- function(a = "a long argument",
                               b = "another argument",
                               c = "another long argument") {
  # As usual code is indented by two spaces.
}
```

These styles are designed to clearly separate the function definition from its body.

```r
# Bad
long_function_name <- function(a = "a long argument",
  b = "another argument",
  c = "another long argument") {
  # Here it's hard to spot where the definition ends and the
  # code begins, and to see all three function arguments
}
```

If a function argument can't fit on a single line, this is a sign you should rework the argument to keep it short and sweet.

#### S7

In S7, the method definition can be long because the function name is replaced by a method call that specifies the generic and dispatch classes. In this case we recommend the single-indent style.

```r
method(from_provider, list(openai_provider, class_any)) <- function(
  provider,
  x,
  ...,
  error_call = caller_env()
) {
  ...
}
```

If the method definition is too long to fit on one line, use the usual rules to spread the method arguments across multiple lines:

```r
method(
  from_provider,
  list(openai_provider, class_any, a_very_long_class_name)
) <- function(
  provider,
  x,
  ...,
  error_call = caller_env()
) {
  ...
}
```

#### `return()`

Only use `return()` for early returns. Otherwise, rely on R to return the result of the last evaluated expression.

```r
# Good
find_abs <- function(x) {
  if (x > 0) {
    return(x)
  }
  x * -1
}
add_two <- function(x, y) {
  x + y
}

# Bad
add_two <- function(x, y) {
  return(x + y)
}
```

Return statements should always be on their own line because they have important effects on the control flow. See also control flow modifiers.

```r
# Good
find_abs <- function(x) {
  if (x > 0) {
    return(x)
  }
  x * -1
}

# Bad
find_abs <- function(x) {
  if (x > 0) return(x)
  x * -1
}
```

If your function is called primarily for its side-effects (like printing, plotting, or saving to disk), it should return the first argument invisibly. This makes it possible to use the function as part of a pipe. `print` methods should usually do this, like this example from httr:

```r
print.url <- function(x, ...) {
  cat("Url: ", build_url(x), "\n", sep = "")
  invisible(x)
}
```

#### Comments

In code, use comments to explain the "why" not the "what" or "how". Each line of a comment should begin with the comment symbol and a single space: `#`.

```r
# Good

# Objects like data frames are treated as leaves
x <- map_if(x, is_bare_list, recurse)


# Bad

# Recurse only with bare lists
x <- map_if(x, is_bare_list, recurse)
```

Comments should be in sentence case, and only end with a full stop if they contain at least two sentences:

```r
# Good

# Objects like data frames are treated as leaves
x <- map_if(x, is_bare_list, recurse)

# Do not use `is.list()`. Objects like data frames must be treated
# as leaves.
x <- map_if(x, is_bare_list, recurse)


# Bad

# objects like data frames are treated as leaves
x <- map_if(x, is_bare_list, recurse)

# Objects like data frames are treated as leaves.
x <- map_if(x, is_bare_list, recurse)
```
