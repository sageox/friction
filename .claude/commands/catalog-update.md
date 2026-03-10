# /catalog-update - Review and update friction catalog

Reviews friction patterns, uses LLM reasoning to categorize them, and updates the CLI's correction catalog.

## When to use

- After a CLI release to review new friction patterns
- Periodically to improve autocorrect coverage
- When dashboards show spikes in unknown commands

---

## Step 1: Fetch pattern candidates

```bash
frictionx catalog build --server "${FRICTIONX_SERVER:-http://localhost:8080}" \
  --catalog default_catalog.json --diff --format table
```

If using an exported JSON file instead of a live server:

```bash
frictionx catalog build --patterns-file patterns.json \
  --catalog default_catalog.json --diff --format table
```

## Step 2: Read current catalog and command tree

```bash
cat default_catalog.json
```

```bash
<your-cli> --help 2>&1
```

## Step 3: Categorize each qualifying pattern

| Category | When | Catalog? | Other action |
|----------|------|----------|--------------|
| `typo` | Misspelling of existing command | Yes — add entry | — |
| `missing-feature` | Users want something that doesn't exist | No | Create issue |
| `agent-hallucination` | AI agents inventing commands/flags | No | Note for docs |
| `noise` | Not actionable | No | — |

### Catalog entry format (for typos)

```json
{
  "pattern": "statu",
  "target": "status",
  "has_regex": false,
  "auto_execute": true,
  "confidence": 0.95,
  "description": "Typo: statu -> status"
}
```

**Confidence rules:**
- 0.95 + auto_execute: single-char typo, pluralization
- 0.90 + auto_execute: clear wrong word, small edit distance
- 0.85 + auto_execute: reasonable inference, unambiguous
- 0.80, no auto_execute: plausible but ambiguous

## Step 4: Update the catalog file

Add new entries to the `commands` array in `default_catalog.json`. Don't remove existing entries.

## Step 5: Summary

Output a table of what was done:

| Category | Count | Examples |
|----------|-------|---------|
| Catalog entries added | X | `statu` -> `status` |
| Missing features | X | `search` (N users) |
| Agent hallucinations | X | `--file` |
| Noise (skipped) | X | one-off typos |
