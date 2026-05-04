# Issue: Function Node – Multi-Output Scaling and Wire Positioning Broken

## Description

When adding additional outputs to the Function node, several visual problems occur:

1. **Uneven output spacing**: New outputs are not positioned with constant spacing relative to each other. The spacing between output ports varies instead of being evenly distributed.

2. **Node height does not scale correctly**: The node height does not adjust proportionally when new outputs are added. The node should grow dynamically in height so that all outputs are displayed with equal spacing.

3. **Output positions shift**: When new outputs are added, the positions of already existing outputs change without existing wire connections being moved along. Already wired connections do not move with the ports, leading to visually broken connections.

## Expected Behavior

- Outputs always have a **constant, even spacing** to each other.
- The **node height grows dynamically** according to the number of outputs.
- When adding/removing outputs, **existing wire connections** are correctly adjusted to the new port positions.

## Steps to Reproduce

1. Create a Function node
2. Add one or more additional outputs
3. Observe: spacing uneven, node height does not adapt
4. Connect a wire to an output, then add another output
5. Observe: wire position no longer matches port position
