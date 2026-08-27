
# Fc Language Reference

## Introduction

Fc is a statically typed programming language with a syntax similar to C, designed for compiling to the NES (6502 processor). This document provides an overview of the language's syntax and features.

## Comments

```c
// This is a single-line comment.

/*
  This is a
  multi-line comment.
*/
```

## Variables and Constants

Variables are declared with the `var` keyword, and constants with the `const` keyword. Type inference is supported, but you can also explicitly specify the type.

### Scopes

You can control the visibility of variables and functions using `public` and `private` scopes.

```c
// Declare a variable with a specific type
var score: int;

// Declare and initialize a variable (type is inferred)
var name = "Player 1";

// Declare a constant
const MAX_LIVES = 3;

// Scoped declarations
public var global_variable: int;
private const SECRET_KEY = 0xAB;
```

## Data Types

Fc supports a variety of data types:

*   **`int`**: A signed integer.
*   **`uint`**: An unsigned integer.
*   **`char`**: A single character.
*   **`bool`**: A boolean value (`true` or `false`).
*   **Arrays**: A collection of elements of the same type.
*   **Pointers**: A reference to a memory address.
*   **Functions**: Can be assigned to variables and passed as arguments.

### Arrays

Arrays are declared with square brackets `[]`.

```c
// An array of 10 integers
var numbers: int[10];

// An array initialized with values
var palette = [0x0F, 0x1F, 0x2F, 0x3F];
```

### Pointers

Pointers are denoted with an asterisk `*`.

```c
var x: int;
var p: int* = &x; // p points to the address of x
*p = 10;          // Dereference p to assign a value to x
```

## Control Flow

### If-Else

Conditional statements use `if`, `elsif`, and `else`.

```c
if (x > 10) {
  // ...
} elsif (x > 5) {
  // ...
} else {
  // ...
}
```

### Loops

Fc provides `while`, `for`, and `loop` for iteration.

```c
// while loop
while (i < 10) {
  i += 1;
}

// for loop (iterates from 0 to 9)
for (i, 0, 10) {
  // ...
}

// Infinite loop
loop {
  // ...
  if (condition) {
    break; // Exit the loop
  }
}
```

### Switch

The `switch` statement provides multi-way branching.

```c
switch (val) {
  case 0:
    // ...
    break;
  case 1, 2: // Multiple values can be specified
    // ...
    break;
  default:
    // ...
}
```

## Functions

Functions are defined with the `function` keyword. You must specify the types of arguments and the return value.

```c
function add(a: int, b: int): int {
  return a + b;
}

// Calling a function
var result = add(5, 3);
```

### Lambdas (Anonymous Functions)

You can create anonymous functions using the `->` syntax.

```c
var my_lambda = ->(x: int): int { return x * 2; };
var result = my_lambda(5); // result is 10
```

## Operators

Fc supports a standard set of operators, similar to C.

*   **Arithmetic**: `+`, `-`, `*`, `/`, `%`
*   **Comparison**: `==`, `!=`, `<`, `>`, `<=`, `>=`
*   **Logical**: `&&`, `||`, `!`
*   **Bitwise**: `&`, `|`, `^`, `<<`, `>>`
*   **Assignment**: `=`, `+=`, `-=`
*   **Pointer**: `&` (address-of), `*` (dereference)

### Type Casting

You can cast an expression to a different type using `<type>expression`.

```c
var my_char = 'A';
var my_int = <int>my_char; // my_int is 65
```

## Modules and Includes

### `use`

The `use` statement imports specific symbols from a module.

```c
// Import the 'print' function from the 'stdio' module
use print from stdio;

// Import 'print' and rename it to 'log'
use print from stdio as log;
```

### `include`

The `include` statement includes the contents of another file.

```c
include "my_library.fc";
```

### `incbin`

The `incbin` function includes a binary file as data.

```c
var sprite_data = incbin("sprite.chr");
```
