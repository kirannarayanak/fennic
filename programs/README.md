# Real programs

Three programs written in Fennic to be kept and used, as M3 step 4 asks.
They are not examples: they don't cover features, they do jobs. What each
one reached for and could not have is recorded in [RECORD.md](RECORD.md).

| Program | Does | Run |
| --- | --- | --- |
| `reconcile/reconcile.fn` | compares a source-layer and a warehouse-layer metric export, joined on metric name, and prints the metrics that differ by more than 3%, largest first, then those on one side only | `fennic run reconcile.fn source.csv warehouse.csv` |
| `connector-audit/audit.fn` | reads every connector JSON in a folder and reports naming, file name, topic prefix and snapshot mode violations | `fennic run audit.fn connectors` |
| `schema-drift/drift.fn` | compares two `information_schema.columns` dumps and reports added, removed and retyped columns per table, treating `int4` and `integer` and similar spellings as the same type | `fennic run drift.fn cluster_a.csv cluster_b.csv` |

Each directory holds sample inputs and the output for them, `*.out`, which
`go test ./...` checks.

Each program exits with 1 when it finds something to report, so a
scheduler or CI can act on it.

The audit reads real Kafka Connect configs, dotted keys and all.
`real-format/` holds the shape the first version could not read; it is
kept as evidence for the record. The audit's name rule is strict: a connector whose name is malformed is not
also checked for its topic prefix, because the prefix rule needs the name's
middle word.
