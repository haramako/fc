
# Fc Compiler Optimization Techniques

## Introduction

The Fc compiler performs several optimization techniques to improve the performance and reduce the size of the generated 6502 assembly code. The optimization level can be controlled by the `:optimize_level` option (default is 2).

## Optimization Techniques

### 1. Constant Folding (Partial)

The compiler simplifies arithmetic operations involving constants:

*   **Power-of-Two Arithmetic**: Multiplication, division, and modulo operations with a constant that is a power of two are replaced with faster shift (`asl`, `lsr`, `rol`, `ror`) and bitwise `and` instructions.
*   **Zero Multiplication**: Any multiplication by zero is resolved to `0` at compile time.
*   **Division by Zero**: An attempt to divide by zero will result in a compile-time error.

### 2. Peephole Optimization for Pointer Access

The compiler optimizes sequences of operations related to pointer and array access.

*   **`index` + `pget`/`pset` Fusion**: A common pattern is accessing an array element to get a pointer, and then immediately dereferencing that pointer to get or set a value. The compiler fuses these two operations (`index` followed by `pget` or `pset`) into a single, more efficient instruction (`index_pget` or `index_pset`). This avoids the creation of a temporary pointer variable and reduces code size and execution time.

### 3. Dead Code Elimination

The `delete_unuse` function removes code that has no effect on the program's output.

*   **Unused Variable Assignments**: Assignments to variables that are never read (`load`, `pget`) are eliminated. The return value of a function call (`call`, `fastcall`) is also discarded if it is not used.

### 4. Register Allocation

The `allocate_register` function optimizes variable storage.

*   **Zero-Page Allocation**: Instead of allocating all local variables on the stack frame, the compiler attempts to assign frequently used variables to zero-page registers. Access to zero-page memory is significantly faster than indexed access to the stack.

### 5. Jump Extension

The 6502 processor's branch instructions (`beq`, `bne`, `bcc`, etc.) have a limited jump range of -128 to +127 bytes. The `extend_jump` function overcomes this limitation.

*   **Branch Chaining**: If a branch target is too far, the compiler inverts the branch condition to jump over a `jmp` instruction. This allows branching to any location in the code, at the cost of a few extra bytes and cycles.

```assembly
; Original code with a long branch
  bne far_label

; Becomes:
  beq @skip
  jmp far_label
@skip:
```
