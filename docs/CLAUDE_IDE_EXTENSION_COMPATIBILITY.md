# Claude IDE Extension Compatibility Fix

## Problem Summary

When using Claude IDE extension v2.1.17+ in Cursor (but not VS Code), the Anthropic API returns 400 errors:

```
API Error: 400 messages.X.content.0.tool_result.content.0: Input tag 'tool_reference' found using 'type' does not match any of the expected tags: 'document', 'image', 'search_result', 'text'
```

And also:

```
API Error: 400 tools.23.custom.defer_loading: Extra inputs are not permitted
```

## Root Causes

### 1. Invalid Tool Definition Fields

Newer Claude IDE extensions include extra fields in tool definitions that are not accepted by the Bedrock/Vertex API:

- `custom` - Contains `defer_loading: true` (part of Anthropic's "Tool Search Tool" beta feature)
- `defer_loading` - May appear at the tool level
- `cache_control` - Caching hints not supported by Bedrock/Vertex

### 2. Invalid Content Types in `tool_result`

The `tool_result` content blocks may include content types that are only valid internally but not accepted by the API:

- `tool_reference` - Used internally by the extension but **NOT** valid for the API

**Valid content types** for `tool_result.content[]` per Anthropic API docs:
- `text`
- `image`
- `document`
- `search_result`

## Solution

### Files Modified

- `/config.yaml` - Both `/bedrock/claude/v1/messages` and `/vertex/claude/v1/messages` endpoints

### Implementation

#### 1. Filter Tool Definitions

Remove invalid fields from each tool definition:

```yaml
let bodyTools = get(body, "tools");
let filteredTools = bodyTools != nil
  ? map(bodyTools, filterOutKeys(#, ["custom", "defer_loading", "cache_control"]))
  : nil;
let toolsObj = filteredTools != nil ? { "tools": filteredTools } : {};
```

#### 2. Filter `tool_result` Content Arrays

For each message, when processing content blocks:
- If the block is `tool_result` with an array `content` field
- Filter to only include items where `type` is in the allowed list
- This removes `tool_reference` and any other invalid types

```yaml
let allowedTypes = ["text", "image", "document", "search_result"];

blockType == "tool_result" && blockContent != nil && type(blockContent) != "string"
  ? merge(filterOutKeys(block, ["content"]), {
      "content": filter(blockContent, get(#, "type") in allowedTypes)
    })
  : block
```

---

## Expr Language Gotchas

The Proximity proxy uses the [expr-lang/expr](https://github.com/expr-lang/expr) Go library for expressions. Here are critical things learned:

### 1. Cannot Redeclare Variables

**DON'T:**
```yaml
let _ = log("first");
let _ = log("second");  # ERROR: cannot redeclare variable _
```

**DO:**
```yaml
# Use unique variable names if you need multiple log statements
let _log1 = log("first");
let _log2 = log("second");

# Or just don't use logging in production code
```

### 2. `type()` Returns Go Type Names, Not "array"

**DON'T:**
```yaml
type(someArray) == "array"  # This will NEVER be true
```

**DO:**
```yaml
# Check if NOT a string and NOT nil instead
someValue != nil && type(someValue) != "string" && type(someValue) != "nil"
```

The `type()` function returns Go type names like `"[]interface {}"` for arrays, not `"array"`.

### 3. Direct Property Access Can Cause Nil Pointer Errors

**DON'T:**
```yaml
#.content           # Can cause nil pointer if # is nil
body.messages       # Can cause nil pointer if body is nil
```

**DO:**
```yaml
get(#, "content")   # Safe - returns nil if # is nil
get(body, "messages")  # Safe - returns nil if body is nil
```

### 4. Always Check for Nil Before Operations

**DON'T:**
```yaml
map(body.messages, ...)  # Crash if body.messages is nil
filter(someArray, ...)   # Crash if someArray is nil
```

**DO:**
```yaml
let messages = get(body, "messages");
messages != nil ? map(messages, ...) : []
```

### 5. Nested Expressions Need Extra Care

Complex nested `map` and `filter` operations are prone to nil pointer errors. Prefer simpler, flatter expressions with explicit nil checks at each level.

### 6. The `??` Operator Works But Be Careful

```yaml
get(body, "messages") ?? []  # Works - provides default empty array
```

However, combine with explicit nil checks for clarity in complex expressions.

---

## Do's and Don'ts

### DO

1. **Use `get()` for safe property access**
   ```yaml
   get(obj, "property")  # Safe
   ```

2. **Check nil before map/filter operations**
   ```yaml
   let arr = get(obj, "arr");
   arr != nil ? map(arr, ...) : []
   ```

3. **Use `filterOutKeys()` to remove unwanted fields**
   ```yaml
   filterOutKeys(obj, ["field1", "field2"])
   ```

4. **Test with actual payloads** - The Claude IDE extension sends complex nested structures

5. **Keep expressions simple** - Break complex logic into multiple `let` statements

6. **Use unique variable names** - Even for throwaway variables like log results

### DON'T

1. **Don't use `let _ = ...` multiple times** - Variable redeclaration error

2. **Don't assume `type(x) == "array"` works** - Use nil/string exclusion instead

3. **Don't access properties directly on potentially nil values**
   ```yaml
   #.content     # Bad
   body.messages # Bad
   ```

4. **Don't forget the bedrock and vertex endpoints need identical fixes** - They're separate configurations

5. **Don't use complex nested ternaries without nil checks at each level**

---

## Testing

1. Run `make run` to start dev server on port 29575
2. Configure Claude IDE extension to use `http://localhost:29575`
3. Trigger a tool call (e.g., Figma MCP tool)
4. Check Proximity dev server logs for any errors
5. Verify no 400 errors from the API

## Verification Checklist

- [ ] No `expr compile error: cannot redeclare variable` errors
- [ ] No `expr run error: runtime error: invalid memory address or nil pointer dereference` errors
- [ ] No `tool_reference` API errors
- [ ] No `defer_loading: Extra inputs are not permitted` errors
- [ ] Tool calls work correctly through the proxy

---

## References

- [Anthropic Messages API](https://docs.anthropic.com/en/api/messages) - Valid content types
- [expr-lang/expr](https://github.com/expr-lang/expr) - Expression language documentation
- Claude Code CHANGELOG - v2.1.17 to v2.1.27 changes introducing these new fields
