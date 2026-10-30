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

The audit's sample connectors spell `connector_class`, `topic_prefix` and
`snapshot_mode` with underscores, because Fennic cannot yet read the dotted
keys of a real Kafka Connect config; `real-format/` shows the real shape.
The audit's name rule is strict: a connector whose name is malformed is not
also checked for its topic prefix, because the prefix rule needs the name's
middle word.
