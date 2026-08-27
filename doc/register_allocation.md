
# Register Allocation in Fc Compiler

## Overview

Register allocation is a critical optimization phase in the Fc compiler. Its primary goal is to assign local variables to the fastest available storage locations: CPU registers (like the A register), zero-page memory, or the stack frame. Effective register allocation significantly improves the performance of the compiled code by minimizing slow memory access.

This process is handled by the `Fc.allocate_register` method, which orchestrates a series of analyses and assignments.

## Process Flow

The allocation process can be broken down into the following steps:

### 1. Live Range Analysis (`calc_live_range` and `LiveRangeCalculator`)

Before allocating any storage, the compiler must understand how each local variable is used throughout a function. This is achieved through live range analysis, a dataflow analysis technique performed by the `LiveRangeCalculator` class.

*   **Control Flow Graph (CFG)**: The calculator is initialized with a CFG of the function, where each instruction is a node. The graph contains information about successors (where execution can go next) and predecessors (where execution could have come from) for each node.
*   **Use-Define Chains**: The compiler first scans the instructions to find where each variable is **defined** (written to) and **used** (read from).
*   **Backward Dataflow Analysis**: The `LiveRangeCalculator` performs a backward dataflow analysis to determine the liveness of a variable at each point in the program. The core logic is as follows:
    1.  A variable is considered **live-in** at a program point if it is used before it is redefined in that block, or if it is **live-out** from that point.
    2.  A variable is **live-out** from a program point if it is **live-in** at any of its successor points.
    3.  This process is repeated iteratively until the liveness information for all points stabilizes.
*   **Calculating the Range**: Once the analysis is complete, the live range for a variable is determined as the span of instructions from its first definition to its last use where it is live. This range (e.g., `min_instruction..max_instruction`) is crucial for the subsequent allocation steps.

### 2. Storage Location Assignment

Once the live ranges are known, the compiler decides where to store each variable based on its usage patterns.

#### a. Stack Frame Allocation

Variables are forced onto the slower stack frame under specific conditions that make them unsuitable for register allocation:

1.  **Crosses a Function Call**: If a variable's live range extends across a `call` instruction (for a non-fastcall function), it must be stored on the stack to preserve its value, as the called function will overwrite CPU and zero-page registers.
2.  **Address is Taken**: If the address of a variable is taken using the `&` operator, it must reside in a stable memory location (the stack) so that pointers to it remain valid.
3.  **Function Arguments**: Arguments passed to a function are initially placed on the stack frame by the caller.

#### b. CPU Register Allocation (Special Cases)

For very short-lived variables, the compiler can perform aggressive optimizations by assigning them directly to CPU registers.

*   **A Register (`accumulator`)**: A variable is assigned to the `A` register if it is defined and then immediately used in the very next instruction. This is common for intermediate results in expressions. This completely avoids any memory access for that variable.
    *   *Example*: In `y = x + 1`, the result of `x + 1` might be temporarily held in `A` and then immediately stored to `y`.

*   **Condition Register (Status Flags)**: The results of comparison instructions (`eq`, `lt`, `not`) are often just used to decide a conditional branch. Instead of storing the boolean result (0 or 1) in a variable, the compiler can directly use the CPU's status flags (Zero, Carry, Negative).
    *   A variable representing the result of a comparison is marked as being in the `:cond` location. The subsequent `if` instruction is then generated as a direct branch based on the corresponding CPU flag (`beq`, `bcs`, `bmi`, etc.), eliminating the need for a temporary boolean variable.

#### c. Zero-Page Register Allocation

Any variable that is not allocated to the stack or a specific CPU register is a candidate for zero-page allocation. The 6502 can access zero-page memory locations (addresses `$00` to `$FF`) more quickly than other memory regions.

### 3. Zero-Page Allocator (`Allocator` Class)

The `Allocator` class is responsible for assigning specific zero-page addresses to variables.

*   **Graph Coloring Analogy**: It uses a greedy algorithm that is analogous to graph coloring. Each variable is a node, and an edge exists between two nodes if their live ranges overlap.
*   **Address Sharing**: The goal is to assign the same "color" (zero-page address) to as many variables as possible, with the constraint that no two variables with overlapping live ranges can share an address.
*   **Minimizing Memory Usage**: This approach minimizes the total amount of zero-page memory required for the function, allowing different variables to share the same memory slots at different times.

### 4. Fastcall Convention

Functions marked with the `fastcall` option use a different convention to pass arguments and return values, which also affects register allocation.

*   **Dedicated Zero-Page Area**: Instead of the stack, `fastcall` functions use a dedicated area of zero-page memory (starting at `FC_FASTCALL_REG`) for arguments, return values, and local variables.
*   **No Stack Frame**: This avoids stack manipulation (`inx`, `dex`) and is generally faster, but it means that `fastcall` functions cannot be recursive and must be used with care.
