# EcoSim — A Three-Month Distributed Systems & Streaming Telemetry Project

A build-in-public engineering project that uses an ecosystem simulator as an **event factory**. The ecosystem is the excuse; the real product is a distributed, event-driven telemetry platform with deep stateful stream processing, a lakehouse, real Kubernetes, and end-to-end observability.

## How to read this guide

- **Part A** is the standing technical reference (System overview, stack, architecture, domain, events, data, repo, dev/deploy, testing, reliability, security, ADRs). Read it once now, return to it per sprint.
- **Part B** is the six-sprint roadmap. Each sprint is a vertical slice with its own backend / data / distributed-systems / frontend / testing / observability work, plus the build-in-public material (content ideas, learning resources, implementation questions, reflection questions) embedded directly in the sprint.
- Everything here is conceptual by design. There is no source code, schema, config, or diagram code — you make the design decisions; this guide frames them.

## Your parameters (what this guide is scoped to)

| Parameter | Value | Consequence |
|---|---|---|
| Time | 15 hrs/week | ~30 hrs/sprint, ~180 hrs total. Ambitious; scope cuts are pre-planned per sprint. |
| Sprint length | 2 weeks | 6 sprints over ~12 weeks. |
| Languages | Go, Java (Flink only), Python, React+TS | Go = sim core + services; Java = Flink DataStream depth; Python = orchestration/analytics; TS = viz. |
| Depth vs breadth | Deep on a few | The deep spines, in priority order: distributed-systems internals (event-driven design, exactly-once, replay, leader-election/fencing, scaling/chaos), then data-platform *infrastructure* (exactly-once ingestion, Iceberg maintenance, DQ/freshness SLOs), then observability. Analytics is intentionally shallow. |
| Story | Distributed-systems / backend (primary) + data-platform infra (secondary); sensor telemetry / anomaly / alerting as the domain frame | Organisms behave like telemetry-emitting devices (Starlink-adjacent). Analytics engineering is deliberately minimized. |
| Role priority | 1) Distributed Systems / Backend, 2) Data Engineering (infra/platform), and explicitly *not* analytics engineering | Drives the re-weighting throughout: S4 is infra not analytics; scaling/chaos is protected, never the buffer. |
| Analytics | Iceberg + Athena (no Snowflake); one minimal dbt mart only | Serverless, pay-per-query, under budget. dbt/marts kept minimal on purpose. |
| Kubernetes | Build for real | k3s on Oracle Cloud always-free ARM. |
| Cloud budget | ≤ $50/month | Free-tier-first: Oracle ARM (k3s), Neon (Postgres), Upstash (Redis), S3+Athena usage (cents at this scale). |

---

# PART A — THE TECHNICAL GUIDE

## 1. System Overview

**What it does.** EcoSim runs a continuously-ticking simulated world of regions, species, organisms, and resources. On every tick, living organisms emit periodic **telemetry** (position, energy, vital-sign analogs) and the world emits discrete **domain events** (births, deaths, predation, disease, migration, resource depletion, environmental shifts). These streams flow through Redpanda into stateful stream processors and into a lakehouse. The simulation is deliberately *not* scientifically rigorous — it is tuned to produce high volume, interesting correlations, ordering challenges, late and out-of-order data, schema drift, and failure modes.

**Engineering capabilities it demonstrates.** Event-driven architecture with event sourcing and CQRS; partitioning, consumer groups, idempotency, dead-letter handling, and replay; exactly-once stateful stream processing with event-time windowing, watermarks, and CEP (Flink/Java); a lakehouse with Iceberg, Athena, Dagster orchestration, dbt transforms, data-quality enforcement and lineage; OLTP read models (Postgres) and an OLAP path (Athena); caching and live fan-out (Redis); real Kubernetes deployment with IaC; and full observability (metrics, logs, distributed traces, SLOs, consumer-lag/DLQ/freshness/data-quality monitoring).

**Hypothetical users.**
- *Sim operators* — drive the world, inject disturbances (disease, drought, an organism going "dark"), watch live state and alerts. They are the authenticated, authorized users and the demo drivers.
- *Analysts* — ask historical/analytical questions over the lakehouse (survival curves, predation networks, resource utilization, disease spread).
- *On-call (you)* — respond to platform incidents: consumer lag, stalled pipelines, failed checkpoints, data-quality breaches.

**Operational use cases.** Live world view; current population and resource dashboards; real-time anomaly alerts (vital anomalies, population crashes, telemetry gaps = "device offline"); operator interventions; on-call incident response.

**Analytical use cases.** Cohort/survival analysis by species; predation and disease-spread network analysis; resource utilization over seasons; population-dynamics modeling; "what changed after the drought" recomputation via time travel/replay.

**Essential vs optional.**
- *Essential spine:* Go sim engine → Redpanda → (Go projectors to Postgres/Redis) + (Flink to Iceberg) → Athena/dbt/Dagster → React viz + Grafana; on k3s; with OTel.
- *Optional extensions (only if time allows, Sprint 6):* a dedicated high-write NoSQL telemetry store (Scylla/ClickHouse), multi-region partitioning, service mesh mTLS, ML-based anomaly baselines, materialized-view read models.

## 2. Explicit Technology Stack

For each: **why · responsibility · interacts with · vs. alternatives · required/optional · runs where · introduced.**

**Go** — *Why:* best fit for the high-throughput, concurrent, deterministic tick loop and for low-overhead single-binary services. *Responsibility:* simulation engine (the write/command side and event source), the operational API/gateway, read-model projectors, CLI/replay tooling, load generator. *Interacts with:* Redpanda (produce/consume), Postgres, Redis, OTel collector. *Vs. alternatives:* over Python for the hot path (GC/throughput, true concurrency); over Java for services (faster iteration, simpler ops, lighter footprint on a small ARM node). *Required.* *Local + cloud.* *Sprint 1.*

**Java** — *Why:* Flink's DataStream API, CEP library, and state/checkpointing story are first-class in Java and lag in PyFlink; you specifically want Flink depth. *Responsibility:* all Flink jobs — stateful enrichment, event-time windowing, anomaly detection, CEP, exactly-once Iceberg sink. *Interacts with:* Redpanda (source), Iceberg/S3 (sink), Redis/Postgres (side outputs/alerts), OTel. *Vs. alternatives:* PyFlink (weaker state/CEP), Kafka Streams (no native lakehouse sink, less windowing power), Spark Structured Streaming (micro-batch, weaker event-time semantics for this use). *Required (and scoped to Flink only — do not write services in Java).* *Local + cloud.* *Sprint 3.*

**Python** — *Why:* the strongest ecosystem for orchestration, analytics engineering, and data quality. *Responsibility:* Dagster orchestration, dbt model execution, data-quality checks, Iceberg maintenance jobs, batch exports, and the load/scenario harness. *Interacts with:* Athena, S3/Iceberg (catalog), Postgres, Redpanda (scenario injection), OTel. *Vs. alternatives:* doing batch glue in Go (worse library ecosystem for analytics/DQ). *Required.* *Local + cloud.* *Sprint 4.*

**React + TypeScript** — *Why:* rich interactive visualization of a spatial, live, event-shaped world; TS for safety over event payloads. *Responsibility:* the world map/live view, the alert console, the operator console, and a thin analytics view that queries the API. *Interacts with:* Go API (REST + WebSocket/SSE). *Vs. alternatives:* server-rendered dashboards only (loses the live spatial demo that makes the YouTube content compelling). *Required (kept intentionally lean).* *Local + cloud.* *Sprint 1 (skeleton), grows each sprint.*

**Redpanda** — *Why:* Kafka API compatibility (all learning transfers) without JVM/ZooKeeper, single binary, far lighter on a small local machine and a small ARM node. *Responsibility:* the event backbone — telemetry topics, domain-event topics, alert topics, DLQs; plus the schema registry. *Interacts with:* every producer/consumer and Flink. *Vs. alternatives:* Apache Kafka (heavier local footprint, more ops for a solo dev). *Required.* *Local + cloud.* *Sprint 1.*

**Apache Flink** — *Why:* the canonical engine for exactly-once stateful stream processing with strong event-time semantics. *Responsibility:* see Java above. *Interacts with:* Redpanda, Iceberg/S3, Redis/Postgres, OTel. *Vs. alternatives:* covered under Java. *Required.* *Local + cloud.* *Sprint 3.*

**Apache Iceberg** — *Why:* open table format giving schema evolution, hidden partitioning, snapshots/time travel, and safe concurrent reads/writes over object storage — the backbone of the replay/recompute story. *Responsibility:* the lakehouse tables (raw telemetry, raw events, curated marts). *Interacts with:* Flink (streaming sink), Athena (query), dbt (transform), Dagster (maintenance). *Vs. alternatives:* Delta/Hudi (Iceberg has the cleanest Athena integration and the strongest open-catalog story). *Required.* *Cloud (S3); locally via MinIO + a local catalog.* *Sprint 4.*

**AWS Athena** — *Why:* serverless, pay-per-query SQL over Iceberg; no cluster to run; cents at this volume. Replaces Snowflake to honor the budget. *Responsibility:* the OLAP/analytical query engine and dbt's warehouse target. *Interacts with:* Iceberg/S3 via Glue catalog, dbt. *Vs. alternatives:* Snowflake (cost), Trino self-hosted (more ops). *Required.* *Cloud; locally substitute DuckDB or Trino against MinIO.* *Sprint 4.*

**AWS S3** — *Why:* cheap, durable object storage = the data lake floor. *Responsibility:* Iceberg data + metadata, exports, checkpoints/savepoints (optionally). *Interacts with:* Flink, Athena, Dagster, dbt. *Vs. alternatives:* none at this price/durability. *Required.* *Cloud; MinIO locally.* *Sprint 4.*

**PostgreSQL** — *Why:* the OLTP read-model and metadata store, plus the transactional outbox. *Responsibility:* current-entity read models, lineage (parent→offspring), alert history, outbox table, Flink/job metadata. *Interacts with:* Go projectors and API, Dagster. *Vs. alternatives:* none needed; it is the relational default. *Required.* *Local (Docker) / hosted Neon free tier.* *Sprint 1.*

**Redis** — *Why:* the hot/live layer and the dedup/idempotency layer. *Responsibility:* latest world snapshot, live population counters, pub/sub fan-out to the frontend, consumer-side dedup sets, cache for expensive read queries. *Interacts with:* Go projectors/API, frontend (via API). *Vs. alternatives:* in-memory map (no persistence/fan-out), Memcached (no pub/sub or data structures). *Required (this is your justified NoSQL/KV store).* *Local (Docker) / hosted Upstash free tier.* *Sprint 2.*

**Dagster** — *Why:* asset-based orchestration whose lineage graph pairs naturally with the data-lineage learning goal, with better local dev ergonomics than Airflow for a solo builder. *Responsibility:* schedule/observe batch — Iceberg compaction & expiry, daily/region rollups, dbt runs, data-quality gates, exports, and on-demand replay/recompute jobs. *Interacts with:* Athena, Iceberg, dbt, Postgres. *Vs. alternatives:* Airflow (heavier, task-centric not asset-centric), Prefect (weaker lineage story). *Required.* *Local + cloud.* *Sprint 4.*

**dbt** — *Why:* declarative ELT with built-in tests and lineage docs; the analytics-engineering keyword done properly. *Responsibility:* transform raw Iceberg tables into curated analytical marts; enforce relationship/freshness/uniqueness tests. *Interacts with:* Athena (target), Dagster (runner), Iceberg. *Vs. alternatives:* hand-written batch SQL jobs (loses tests/lineage/docs). *Required-ish (high value, modest cost).* *Local (dbt+DuckDB) / cloud (dbt+Athena).* *Sprint 4.*

**Docker / Docker Compose** — *Why:* the entire system must run on your laptop for development and demos. *Responsibility:* local orchestration of every component. *Required.* *Local.* *Sprint 1.*

**Kubernetes (k3s)** — *Why:* you want real orchestration experience; k3s on Oracle Cloud always-free ARM gives it at $0. *Responsibility:* hosted deployment of services, Flink, and observability. *Interacts with:* everything deployed. *Vs. alternatives:* EKS (over budget), Nomad (less industry signal). *Required.* *Cloud (and optionally a local single-node k3s/kind to rehearse).* *Sprint 5.*

**Terraform** — *Why:* declarative IaC for the AWS surface (S3, Glue catalog, Athena workgroup, IAM) and reproducibility. *Responsibility:* cloud resource provisioning; Helm/Kustomize handle in-cluster objects. *Vs. alternatives:* Pulumi (fine too; you've used both — pick one and write the ADR). *Required.* *Cloud.* *Sprint 5.*

**OpenTelemetry + Prometheus + Grafana + Tempo (or Jaeger) + Loki** — *Why:* the observability spine across a polyglot, async system; OTel gives one instrumentation standard, Prometheus/Grafana the metrics+dashboards, Tempo/Jaeger the traces (critical across the async Redpanda/Flink hops), Loki the logs. *Responsibility:* metrics, traces, logs, dashboards, alerts, SLOs. *Interacts with:* every service and job. *Vs. alternatives:* vendor APM (cost), ad-hoc logging (no correlation). *Required, introduced early and grown every sprint — never a final-week task.* *Local + cloud.* *Sprint 1 onward.*

## 3. Architecture

**Bounded contexts (and why they are not all microservices).** Four contexts, deployed as a *small* number of services — the prompt's "don't make everything a microservice" is honored by collapsing related responsibilities:

1. **Simulation context** — the world engine. *One service* (Go). Owns authoritative state and is the event source. Command side of CQRS.
2. **Stream-processing context** — Flink jobs. *One logical deployment* (Java). Stateful enrichment, anomaly detection, CEP, lakehouse sink.
3. **Serving/query context** — the operational API + projectors. *One service* (Go) that both projects read models (Postgres/Redis) and serves the frontend. Read side of CQRS.
4. **Analytics/lakehouse context** — Dagster + dbt + Athena/Iceberg. *Batch plane*, not a request-serving service.

Observability and the frontend are cross-cutting, not contexts.

**Data ownership.** The simulation context owns the event log (source of truth). No other component writes domain truth; they only derive from it. Postgres/Redis are *owned by* the serving context as derived read models. Iceberg is owned by the analytics context as the derived historical store. This single-writer-per-store rule is the backbone of the whole design.

**Event flows (synchronous vs asynchronous).**
- *Async (the spine):* engine → Redpanda topics → {Go projectors → Postgres/Redis} and {Flink → Iceberg + alert topic}. Alert topic → projector → Redis → frontend.
- *Sync:* frontend → Go API (REST for queries/commands; WebSocket/SSE for live push fed from Redis pub/sub). Operator commands → API → engine (command topic or direct command with outbox).
- *Batch:* Dagster triggers dbt/maintenance/DQ jobs reading and writing Iceberg via Athena.

**Operational vs analytical paths.** Operational (low-latency): Redpanda → projectors → Redis/Postgres → API → frontend, and Flink → alerts → frontend. Analytical (high-latency, high-volume): Redpanda → Flink → Iceberg → Athena/dbt → marts → analytics view. The Flink sink is the bridge from the operational stream to the analytical lake.

**Authn/authz boundaries.** The frontend and API are the only externally exposed surface; they enforce user authentication (OIDC/JWT) and role-based authorization (operator vs analyst vs read-only). Internal service-to-service traffic lives inside the cluster network and is hardened in Sprint 5 (network policies, optional mTLS). The engine accepts operator commands only via the authenticated API, never directly from the internet.

**Failure handling (system level).** Producer retries + idempotent producer; idempotent consumers (dedup by event id); DLQs per consumer for poison messages; Flink checkpointing for exactly-once recovery; replay tooling to rebuild any read model from the log; Iceberg snapshots for time travel and safe concurrent writes; Dagster retries/backfills for batch.

**Deployment model.** Local: Docker Compose, one network, MinIO standing in for S3, local catalog/DuckDB or Trino standing in for Athena. Hosted: k3s on Oracle ARM for the services + Flink + observability; AWS for S3/Glue/Athena. Clear seam: anything stateful-and-cheap-managed (Postgres, Redis) uses free hosted tiers (Neon, Upstash); anything you want to operate (Redpanda, Flink, services, Grafana stack) runs on k3s.

**Observability model.** Every service emits OTel traces and metrics to a collector; metrics scraped by Prometheus; traces to Tempo/Jaeger; logs to Loki; Grafana unifies. Trace context propagates *through* Redpanda headers so a single operator command can be followed end-to-end across async hops — this is the showpiece.

**Diagrams to create (descriptions only — build them yourself):**
1. *Context map* — the four bounded contexts as boxes, with the event backbone as a central spine; arrows labeled async/sync/batch; the single-writer ownership annotated on each store.
2. *Event flow / dataflow* — left-to-right: engine → topics (telemetry, domain, command, alert, DLQ) → the two parallel consumer paths (projectors; Flink) → stores → API/Grafana/frontend.
3. *CQRS + event sourcing* — write side (engine + outbox + event log) vs read side (projectors + Postgres/Redis + Iceberg), with the replay arrow re-feeding projectors.
4. *Deployment topology (two panels)* — local Docker Compose vs k3s+AWS hosted, showing what moves where and the free-tier seams.
5. *Flink job internals* — sources → keyed state → event-time windows + watermarks → anomaly/CEP operators → side outputs (alerts) + Iceberg sink + checkpoint barrier.
6. *Trace waterfall* — one operator command traced across API → engine → topic → projector, illustrating context propagation.

## 4. Domain Model

The domain stays small enough to manage but rich enough to generate hard problems.

**Bounded contexts → aggregates.**

*Simulation context aggregates:*
- **Region (Habitat)** — the consistency boundary for environment and local population. Holds environmental conditions and resource nodes. *Invariants:* resource levels ≥ 0; population ≤ carrying capacity (soft, drives migration/death); season transitions are monotonic within a cycle.
- **Organism** — the per-entity aggregate and the primary telemetry emitter. Holds species reference, region, age, energy, vitals, position, lifecycle state. *Invariants:* energy within `[0, max]`; a dead organism emits no further telemetry or actions; an organism exists in exactly one region at a time; offspring inherit a parent reference (lineage).
- **ResourceNode** — food/water within a region; depletes on consumption, regenerates on tick. *Invariant:* never negative; regeneration capped.

*Reference / value objects:*
- **Species** — traits: metabolism rate, reproduction threshold, predator/prey relations, disease susceptibility, telemetry cadence. Reference data, slowly changing.
- **Position**, **Vitals** (energy, stress, a heart-rate analog), **EnvironmentalConditions** (temperature, moisture, season), **EnergyLevel** — immutable value objects.

**Commands (write side, all flow through the engine).** `AdvanceTick` (self-driven clock), `SpawnOrganism`, `ApplyEnvironmentalShift`, `InjectDisease`, `InjectDrought`, `SilenceOrganismTelemetry` (operator-triggered "device goes dark" — the telemetry-gap demo), `InjectAnomaly` (failure-mode trigger). Operator commands are authenticated; tick is internal.

**Domain events (discrete, lower volume, high meaning).** `OrganismBorn`, `OrganismMoved`, `OrganismFed`, `OrganismEnergyChanged`, `PredationOccurred`, `OrganismDied{cause}`, `DiseaseContracted`, `DiseaseSpread`, `MigrationStarted/Completed`, `ResourceDepleted/Regenerated`, `EnvironmentalConditionChanged`, `SeasonChanged`, `PopulationMilestoneReached`, `ExtinctionOccurred`.

**Telemetry events (continuous, high volume — the "sensor" stream).** `OrganismTelemetryTick` (per living organism, per cadence: position, energy, vitals, region) and `RegionEnvironmentTick`. These dominate volume and are the anomaly-detection substrate.

**Read models (derived, owned by serving/analytics).**
- *Postgres:* current organisms, current regions/resources, lineage tree, alert history, population-by-region snapshots.
- *Redis:* live world snapshot, live counters, frontend pub/sub channel, dedup/idempotency sets.
- *Iceberg/Athena:* full historical telemetry and events; curated marts (survival cohorts, predation network, disease-spread edges, resource utilization, population dynamics).

**Relationship between simulation behavior and generated data.** Reproduction → birth events + a new telemetry emitter (volume grows). Predation/disease → correlated event bursts + sudden vital changes (anomaly fodder + CEP patterns). Migration → partition-key churn (a moving organism crosses region boundaries → ordering and routing questions). Environmental shifts → population crashes (windowed-aggregate anomalies). Operator silencing → telemetry gaps (offline-detection). Extinction → cohort/survival analytics. Each behavior is chosen because it stresses a specific platform capability, not for ecological realism.

## 5. Event Model

Major categories, conceptual fields only (no schemas).

**A. Telemetry events** — *Produces:* engine, per living organism per cadence. *Consumes:* Flink (primary), a live projector (sampled, for the frontend). *Partition key:* organism id (per-entity ordering; even spread; enables Flink keyed state per organism). *Ordering:* per organism only (global ordering neither needed nor affordable). *Delivery:* at-least-once into the topic; exactly-once into Iceberg via Flink checkpointing; the live projector is idempotent/last-write-wins. *Idempotency:* event id + monotonic per-organism sequence; consumers dedup. *Retention:* short on the topic (hours–days) since the lake is the system of record; size for replay-into-Flink, not long-term storage. *Replay:* re-read recent window to rebuild Flink state or live snapshot; long-range replay comes from Iceberg, not the topic. *Schema evolution:* additive fields (new vital) must be backward compatible; this is where you practice registry-enforced compatibility. *DQ risks:* missing ticks (gap vs genuine offline), duplicate ticks, out-of-order arrival, clock skew between sim time and processing time. *Op value:* live view, gap/offline alerts. *Analytical value:* the raw substrate for every time-series mart.

**B. Lifecycle domain events** (born/died/moved/fed/energy-changed) — *Produces:* engine. *Consumes:* Postgres projector (current state + lineage), Flink (correlation), analytics sink. *Partition key:* organism id (keeps an organism's lifecycle ordered on one partition). *Ordering:* per organism strict (born before moved before died). *Delivery:* at-least-once + idempotent projector. *Idempotency:* event id dedup; state transitions must be order-tolerant where possible. *Retention:* medium on topic; permanent in Iceberg. *Replay:* full Postgres read-model rebuild from Iceberg/topic. *Schema evolution:* `cause` enums on death will grow — forward/backward compatibility drill. *DQ risks:* a "died" without a preceding "born" (lineage gap), reordering across a migration. *Op value:* current populations, lineage. *Analytical value:* survival/cohort analysis.

**C. Interaction events** (predation, disease contracted/spread, competition) — *Produces:* engine. *Consumes:* Flink CEP (outbreak/cascade detection), analytics sink. *Partition key:* region id (interactions are local; co-locating a region's interactions enables regional CEP without cross-partition shuffles) — *note the deliberate tension with organism-id keying; document the tradeoff.* *Ordering:* per region. *Delivery:* at-least-once. *Idempotency:* event id; CEP must tolerate duplicates. *Retention:* medium/permanent in lake. *Replay:* re-feed CEP to validate detection determinism. *Schema evolution:* new interaction types. *DQ risks:* hotspot partitions when one region dominates; orphaned "spread" referencing an unknown "contracted." *Op value:* outbreak alerts. *Analytical value:* predation/disease network graphs.

**D. Environment events** (condition changed, season changed, resource depleted/regenerated) — *Produces:* engine. *Consumes:* Flink (windowed context), projectors, analytics. *Partition key:* region id. *Ordering:* per region (season monotonicity). *Delivery:* at-least-once. *Idempotency:* event id. *Retention:* medium/permanent. *Replay:* environment timeline reconstruction. *Schema evolution:* additive condition metrics. *DQ risks:* season regressions (invariant violation surfaced as a DQ alert). *Op value:* environment dashboard. *Analytical value:* "what changed after the drought" recomputation.

**E. Command events** (operator interventions) — *Produces:* API (authenticated). *Consumes:* engine. *Partition key:* target region/organism id. *Ordering:* per target. *Delivery:* at-least-once with outbox to avoid dual-write; engine dedups by command id. *Idempotency:* command id; commands are designed idempotent where possible. *Retention:* short. *Replay:* generally *not* replayed (commands are intents, not facts) — important conceptual distinction to document. *Schema evolution:* new command types. *DQ risks:* duplicate command application. *Op/analytical value:* audit trail of interventions.

**F. Alert events** (emitted by Flink) — *Produces:* Flink anomaly/CEP operators. *Consumes:* alert projector → Redis → frontend; Alertmanager bridge; analytics sink (alert history). *Partition key:* region or organism id. *Ordering:* not strict (alerts are independent). *Delivery:* at-least-once; UI dedups by alert id + window. *Idempotency:* alert id keyed on (rule, entity, window). *Retention:* medium; permanent history in Postgres/Iceberg. *Replay:* re-derive alerts from replayed telemetry to test rule stability. *Schema evolution:* new rule types/severities. *DQ risks:* alert storms (missing suppression), flapping. *Op value:* the core operational signal. *Analytical value:* alert precision/recall over replays.

**G. Dead-letter events** (per consumer) — *Produces:* any consumer that cannot process a message after retries. *Consumes:* a DLQ monitor/projector + you, on-call. *Partition key:* inherit original. *Ordering:* n/a. *Delivery:* at-least-once. *Idempotency:* original event id preserved. *Retention:* long enough to investigate/replay. *Replay:* reprocess after a fix. *DQ risks:* silent DLQ growth (must be monitored/alerted). *Op value:* poison-message visibility. *Analytical value:* failure-rate analysis.

## 6. Data Architecture

**Operational databases (OLTP).** Postgres holds the request-servable read models and the outbox. It answers "what is true *right now*" with strong consistency for current entities, lineage, and alert history. It is rebuilt from the event log on demand.

**NoSQL / caching.** Redis is the live tier: latest world snapshot for instant frontend loads, real-time counters, pub/sub fan-out, and the dedup/idempotency sets that make consumers safe. A dedicated wide-column/time-series store (Scylla/ClickHouse) for high-write raw telemetry is an *optional* Sprint-6 extension — only added if load testing shows Iceberg-via-Flink can't serve a "recent raw telemetry" query need; do not add it preemptively.

**Event backbone.** Redpanda carries all categories above, with separate topics per category and per-consumer DLQs, and the schema registry enforcing compatibility.

**Stream processing.** Flink is the stateful heart: keyed state per organism/region, event-time windows with watermarks for rolling aggregates, anomaly detection against learned baselines, CEP for outbreak/predation cascades, side outputs for alerts, and an exactly-once Iceberg sink.

**Orchestration.** Dagster schedules and observes the batch plane as software-defined assets (each mart/maintenance step is an asset with explicit upstream lineage), runs dbt, gates on data quality, and executes on-demand replay/recompute backfills.

**Object storage + table format.** S3 (MinIO locally) stores Iceberg data + metadata. Iceberg gives schema evolution, hidden partitioning, snapshot isolation, and time travel — the substrate for replay/recompute and for safe concurrent Flink-write/Athena-read.

**Warehouse / analytical engine.** Athena queries Iceberg serverlessly; it is also dbt's target. DuckDB/Trino substitute locally.

**Transformation layer.** dbt builds curated marts from raw Iceberg tables (ELT — load raw first, transform in-warehouse), with tests and lineage docs.

**Batch vs streaming ingestion.** Streaming: Flink → Iceberg (continuous, exactly-once). Batch: Dagster-driven compaction, rollups, exports, and recomputations. The boundary — what is computed in-stream vs in-batch — is a deliberate ADR.

**ETL vs ELT.** The lake path is ELT (land raw, transform with dbt). The operational projectors are closer to ETL (transform-on-write into purpose-shaped read models). Holding both in one system is a teaching point.

**Data-quality checks.** Freshness (is the latest partition recent?), schema conformance (registry + dbt), completeness (telemetry gaps), referential integrity (every "died" has a "born"), distribution/anomaly (population within expected band), and invariant checks (no season regression, no negative resources). Failures raise alerts and can block downstream Dagster assets.

**Data lineage.** Dagster asset graph + dbt docs give column/table lineage; trace IDs carried in events give per-record provenance from emission to mart.

**Analytical models (marts).** Survival/cohort by species; predation network (predator→prey edges + weights); disease-spread graph over time; resource-utilization-by-season; population-dynamics time series; alert-quality (precision/recall vs replayed ground truth).

**OLTP vs OLAP.** OLTP = Postgres/Redis, point lookups and current state, low latency. OLAP = Athena over Iceberg, large scans and aggregations, latency-tolerant. The same domain events feed both via different consumers — the canonical CQRS payoff.

**Historical replay & recomputation.** Two flavors: (1) *operational replay* — re-feed recent topic data to rebuild a read model or Flink state after a bug; (2) *analytical recompute* — re-run dbt/Dagster over Iceberg time-travel snapshots to answer "what would the marts say if we fixed this transform?" Both are first-class, not afterthoughts.

**Example operational questions the platform answers.** Which organisms went dark in the last 5 minutes (offline detection)? What is the current population per region vs carrying capacity? Which regions have active outbreak alerts? What did operator X change in the last hour (audit)?

**Example analytical questions.** What is the 7-day survival rate by species, and how did it shift after the drought? Which predator-prey pairs drive the most deaths? How fast does a disease propagate across regions, and does cadence/partitioning affect detection latency? What is resource utilization by season, and where does carrying capacity bind? How stable are alert rules across a deterministic replay (precision/recall)?

## 7. Monorepo Structure

A single repo with clear language and concern boundaries. Tree below describes responsibilities only — no code lives in this guide.

```
ecosim/
├─ README.md                  # what/why, quickstart, architecture links
├─ docs/
│  ├─ adr/                    # one file per ADR (see §12); decision records only
│  ├─ architecture/           # the diagrams you draw + narrative
│  ├─ runbooks/               # incident procedures (grown each sprint)
│  └─ domain/                 # domain model, event catalog, glossary
├─ services/
│  ├─ engine/                 # Go. Simulation engine + event source + outbox. MUST own world truth; MUST NOT query read models.
│  ├─ serving/                # Go. API + read-model projectors + WebSocket. Owns Postgres/Redis read models. MUST NOT mutate world truth.
│  └─ tooling/                # Go. CLI: replay, scenario injection, topic admin. Operational, not request-serving.
├─ stream/
│  └─ flink-jobs/             # Java. DataStream jobs only. Stateful enrichment, anomaly, CEP, Iceberg sink. MUST NOT contain business "world" logic — it derives, never authors.
├─ data/
│  ├─ dagster/                # Python. Asset defs, schedules, sensors, DQ gates, replay/backfill jobs.
│  ├─ dbt/                    # dbt models/tests/docs. Curated marts from raw Iceberg. MUST NOT define raw ingestion.
│  └─ quality/                # Python. Reusable DQ checks/expectations shared by Dagster.
├─ web/
│  └─ console/                # React+TS. World view, alert console, operator console, analytics view. Talks only to serving API.
├─ platform/
│  ├─ compose/                # Local Docker Compose stacks + MinIO/Redpanda/observability. Local only.
│  ├─ terraform/              # AWS surface: S3, Glue, Athena workgroup, IAM. Cloud only.
│  ├─ k8s/                    # Helm/Kustomize for k3s: services, Flink, observability. No app code.
│  └─ observability/          # Dashboards, alert rules, OTel collector config (as data, not app code).
├─ contracts/
│  ├─ schemas/                # Event/command schema definitions (registry source of truth). Owned cross-cutting; changes are reviewed.
│  └─ openapi/                # API contract for the serving service + the frontend.
├─ shared/
│  └─ go-libs/                # Go: producer/consumer helpers, OTel setup, idempotency utils. MUST stay domain-agnostic.
├─ test/
│  ├─ integration/            # cross-service tests (testcontainers): topic→projector→store.
│  ├─ e2e/                    # command→world→event→read-model→UI assertions.
│  └─ load/                   # load/scenario generators + results.
└─ .github/workflows/         # CI: build, unit/integration, lint, schema-compat checks, image publish.
```

**Configuration management.** Per-service config via environment + a single typed config layer; secrets never in the repo (see §11). `contracts/` is the single source of truth for schemas and API shape; services depend on it, not on each other's internals. **Testing** is split by scope (`test/` for cross-service; unit tests live beside each service). **Deployment artifacts** live in `platform/` and are environment-segregated (compose = local, terraform+k8s = cloud) so the local/cloud seam is explicit and never tangled.

## 8. Local Development and Deployment

**Starting locally.** A single Compose command brings up Redpanda (+ registry + console), MinIO (S3), Postgres, Redis, the OTel collector + Prometheus + Grafana + Tempo + Loki, then the engine, serving service, and (from Sprint 3) a local Flink, plus the web console. A Makefile/Taskfile wraps the common flows (up, seed, replay, down).

**What runs in Docker.** Everything in local mode. Cloud substitutes appear only when hosted: MinIO→S3, local catalog/DuckDB→Glue/Athena, local Postgres/Redis→Neon/Upstash.

**Dependency initialization.** An init step creates topics with chosen partition counts, registers schemas, runs Postgres migrations, creates Iceberg namespaces/tables (or lets Flink create them), and provisions Grafana dashboards/datasources from `platform/observability` as data.

**Test-data generation.** The scenario harness (Python) and engine "scenario mode" seed deterministic worlds (fixed seed → reproducible event streams), and can dial volume up for load tests and inject disturbances (drought, outbreak, silenced organisms).

**Service discovery.** Local: Compose DNS. Cloud: k8s Services + DNS; external managed stores via configured endpoints/secrets.

**Secrets & configuration.** Local: a git-ignored env file. Cloud: k8s Secrets sourced from SOPS-encrypted files or External Secrets; AWS access via least-privilege IAM (IRSA-style binding or scoped keys). No plaintext secrets anywhere in the repo.

**Database migrations.** A migration tool runs Postgres schema changes idempotently on startup/deploy; Iceberg schema changes go through registry-compatible evolution, not destructive rewrites.

**Local observability.** Grafana preloaded with the same dashboards used in prod, fed by the local Prometheus/Tempo/Loki — so you debug locally with production-shaped telemetry from day one.

**Path to Kubernetes/cloud.** Containers are built identically for local and cloud; the only differences are config and the managed-store endpoints. Sprint 5 introduces k3s on Oracle ARM, Helm/Kustomize for in-cluster objects, and Terraform for the AWS surface. **IaC approach:** Terraform for cloud resources, Helm or Kustomize for cluster objects — pick one of Terraform/Pulumi and one of Helm/Kustomize and record both choices as ADRs.

## 9. Testing Strategy

- **Unit** (every sprint, beside each service): pure domain logic — invariants, energy math, tick determinism, projector reducers, idempotency keys. Heaviest in Sprints 1–2.
- **Integration** (Sprint 2+, testcontainers): topic→projector→store; producer→consumer dedup; outbox→publish. Real Redpanda/Postgres/Redis in containers.
- **Contract** (Sprint 2+): schema-registry compatibility checks in CI; API contract tests against `contracts/openapi`. Prevents a producer change from silently breaking consumers.
- **End-to-end** (Sprint 3+): command→world→event→read-model→UI; alert path from injected anomaly to console.
- **Data-quality tests** (Sprint 4+): dbt tests + Dagster DQ gates (freshness, completeness, referential, distribution, invariants).
- **Property-based** (Sprint 2+): generate random valid command sequences; assert invariants hold and replayed state converges (great fit for the deterministic engine).
- **Load** (Sprint 6, harness built earlier): scale telemetry volume; measure consumer lag, Flink backpressure, p99 latency, throughput ceilings.
- **Failure-injection** (Sprints 2–6): kill consumers/pods, drop a broker, corrupt a message (→ DLQ), induce checkpoint failure, partition the network; assert recovery and no data loss beyond stated guarantees.
- **Replay/determinism** (Sprint 2+): same seed + same event log ⇒ identical read models and identical alert sets; the core correctness guarantee.
- **Schema-compatibility** (Sprint 2+, intensified Sprint 4): deliberately evolve a schema and prove old/new consumers coexist.

## 10. Reliability and Observability

- **Logs** — structured, correlation-id/trace-id tagged, shipped to Loki.
- **Metrics** — RED (rate/errors/duration) for services; USE (utilization/saturation/errors) for resources; domain metrics (events/sec, live population, alerts/min); Prometheus-scraped.
- **Distributed traces** — OTel context propagated through Redpanda headers so one operator command is followable across API→engine→topic→projector→Flink; Tempo/Jaeger.
- **Dashboards** — per-service health; pipeline overview (produce→consume→sink); lag/DLQ/freshness; data-quality status; business view (population, alerts).
- **Alerts** — consumer lag over threshold, DLQ growth, Flink checkpoint failures/restarts, pipeline freshness SLO breach, data-quality gate failure, error-rate spikes; routed via Alertmanager.
- **SLIs** — end-to-end event latency (emit→read-model), alert latency (anomaly→console), pipeline freshness (event time→queryable in Athena), API availability/latency, data-quality pass rate.
- **SLOs** — define explicit targets per SLI (e.g., a freshness budget and an alert-latency budget) with error budgets; documented in `docs/runbooks`.
- **DLQ monitoring** — count + age + sample of poison messages; an alert if non-empty beyond a grace window.
- **Consumer-lag monitoring** — per-group lag, with the offline-organism gap detector as a domain-level "lag" analog.
- **Pipeline-freshness monitoring** — event-time vs processing-time skew; Iceberg latest-partition recency.
- **Data-quality monitoring** — DQ check results as time series with alerting.
- **Recovery procedures & runbooks** — per failure class: stuck consumer (inspect lag → reset offset/replay), DLQ backlog (diagnose → fix → reprocess), failed checkpoint (restore from savepoint), stale pipeline (find blocked asset → backfill), DQ breach (quarantine partition → recompute).
- **Incident simulations** — scheduled chaos exercises (Sprints 5–6) that you run, observe, and write up as postmortems (also content).

## 11. Security

- **Authentication** — users authenticate to the API/frontend via OIDC/JWT (a hosted free IdP or a self-hosted Keycloak on k3s). Tokens verified at the API edge.
- **Authorization** — RBAC: *operator* (issue commands), *analyst* (read analytics), *viewer* (read live). Enforced in the serving service; commands require operator scope.
- **Service-to-service** — inside the cluster, traffic is constrained by network policies; optional mTLS (linkerd) as a Sprint-6 stretch. Internal services are never internet-exposed.
- **Secret management** — SOPS-encrypted secrets or External Secrets into k8s Secrets; least-privilege IAM for AWS; no secrets in git or images.
- **API security** — input validation against `contracts/openapi`, rate limiting, auth␣z on every mutating route, audit logging of operator commands.
- **Data access** — analysts query through the API/Athena workgroup, not raw S3; bucket policies least-privilege; PII is irrelevant here (synthetic data), which you note as the reason real-world hardening (encryption at rest beyond defaults, row-level access) is *designed* but not *exhaustively* implemented.
- **Audit events** — every operator command produces an immutable audit record (command-event stream + Postgres history).
- **Dev vs prod boundaries** — dev uses static local secrets and open dashboards; prod uses encrypted secrets, authenticated Grafana, network policies, and least-privilege IAM. The seam is explicit in `platform/`.

## 12. Architecture Decision Records (questions only)

Each ADR answers a question; write the decision yourself.

1. **Service boundaries** — How few services can we run while keeping single-writer ownership and clear contexts? When does collapsing two responsibilities into one service become wrong?
2. **Language selection** — Why Go for engine/services, Java strictly for Flink, Python for the data plane? Where would crossing that line cost us?
3. **Backbone choice** — Why Redpanda over Kafka given identical APIs, and what do we lose by not running real Kafka?
4. **Topic design** — How many topics, split by event category vs entity? What are the naming, partition-count, and retention conventions?
5. **Partitioning strategy** — Organism-id vs region-id keying: which categories use which, and how do we handle the migration/hotspot tension?
6. **Delivery semantics** — Where is at-least-once + idempotency sufficient, and where do we pay for exactly-once (Flink→Iceberg) and why?
7. **Event sourcing & CQRS** — Is the event log the system of record? What can and cannot be replayed (facts vs commands)?
8. **Outbox** — Why an outbox for commands, and where is it unnecessary because the engine already appends-then-publishes?
9. **Storage selection** — Why Postgres + Redis + Iceberg specifically, and what is each forbidden from owning?
10. **Iceberg table design** — Partitioning/sort layout, snapshot/expiry policy, and the compaction cadence vs query-latency tradeoff.
11. **Stream vs batch boundary** — What is computed in Flink vs in dbt/Dagster, and why?
12. **Orchestration** — Why Dagster (asset-centric) over Airflow for the lineage goal?
13. **Deployment** — Why k3s on Oracle ARM over EKS, and what production realism are we knowingly trading for cost?
14. **IaC** — Terraform vs Pulumi; Helm vs Kustomize; why each pick?
15. **Observability stack** — Why OTel + Prometheus/Grafana/Tempo/Loki, and why propagate trace context through Redpanda headers?
16. **Schema evolution** — Which compatibility mode (backward/forward/full) and why; how do we stage a breaking change?
17. **Local-vs-cloud** — Which components are managed (Neon/Upstash/Athena) vs self-operated (Redpanda/Flink/services), and what principle drives the split?
18. **Anomaly strategy** — Rule/threshold + CEP vs learned baselines: why start simple, and what would justify ML later?
19. **Engine singleton coordination** — How do we guarantee exactly one engine writes truth at a time? Which leader-election mechanism (k8s Lease vs DB/Redis lock), what lease duration, and how do fencing tokens prevent a stale leader from committing after losing leadership?

---

# PART B — THE THREE-MONTH ROADMAP

Six two-week sprints (~30 hrs each). Every sprint is a vertical slice: it touches infra, backend, data, observability, and frontend, and ends in something you can demo and film. Sprints build strictly on each other.

> **A note on your 15 hrs/week.** This is tight for the depth you want. Each sprint lists explicit scope reductions; take them early rather than slipping. Sprint 6 is intentionally a buffer/hardening sprint — let earlier slip land there.

---

## Sprint 1 (Weeks 1–2) — Walking Skeleton: the world breathes and you can see it

**Objective.** Stand up the thinnest end-to-end slice: a Go engine that ticks and emits events to Redpanda, a Go projector that builds current state in Postgres, a bare React view that shows the world updating, and observability + CI scaffolding from day one.

**User/system-visible outcome.** Open the browser, see a grid of regions/organisms updating every tick; open Grafana, see events/sec and live population.

**Functional requirements.** Engine advances ticks on a clock; organisms have position/energy and move/feed; emits born/moved/died + telemetry; projector maintains current organisms/regions in Postgres; API serves current state; frontend renders it.

**Infrastructure.** Docker Compose: Redpanda (+console+registry), Postgres, OTel collector, Prometheus, Grafana. Makefile/Taskfile. GitHub Actions: build + unit tests + lint.

**Backend.** Engine core (deterministic tick, seedable); a Go producer helper in `shared/go-libs`; the serving service skeleton (projector + REST). Topics created with deliberate partition counts.

**Data-platform.** Postgres migrations; the current-state read model; topic + retention conventions (first cut).

**Distributed-systems concepts.** Producing to a partitioned topic; partition-key choice (organism id) and why; consumer offset basics; the idea that the topic — not Postgres — will become the source of truth.

**Frontend/visualization.** Minimal React+TS grid polling the API; a population counter. No styling polish.

**Testing.** Unit: tick determinism, movement/energy rules, projector reducer. CI runs them.

**Observability.** OTel in engine + serving; Prometheus scraping; one Grafana dashboard (events/sec, population, basic service health). This is the "observability is never a final task" commitment, honored in week one.

**Failure scenarios.** Restart the projector and confirm it resumes consuming; stop Postgres and observe the projector's behavior (and decide what *should* happen).

**ADRs to write.** #2 language selection, #3 backbone choice, #4 topic design (first cut), #5 partitioning (first cut).

**Definition of done.** `make up` yields a visibly ticking world in the browser and live metrics in Grafana; CI green; the four ADRs drafted.

**Deliverable/demo.** A 60-second screen capture: world ticking + Grafana counters climbing.

**Tech debt deferred.** No event sourcing yet (Postgres is still effectively source of truth — fixed in S2); no idempotency; no schema registry enforcement; no Redis/live push (polling only).

**Risks & scope reductions.** *Risk:* yak-shaving the Compose/observability setup. *Reductions:* drop Loki/Tempo this sprint (metrics only); single region; ≤3 species; polling instead of WebSocket.

**Content ideas.**
- *"I'm building a distributed system disguised as a fish tank — here's the plan."* Lesson: why an ecosystem is a great event factory. Demo: the architecture diagram + first ticking grid. Problem: turning a fun domain into real engineering. Valuable because it frames the whole series with a clear engineering thesis, not a toy.
- *"Day-one observability: metrics before features."* Lesson: instrument first so you debug with data, not print statements. Demo: Grafana lighting up as the engine starts. Problem: most side projects bolt on monitoring last. Valuable because it models a senior habit juniors skip.

**Learning resources.** DDIA ch.1 (reliability/scalability/maintainability framing); Redpanda quickstart docs; OTel "what is a trace/metric" concepts; *The Twelve-Factor App* (config/processes). Keep it light — you're mostly building.

**Implementation questions.** What is a "tick" — wall-clock or logical time, and how will that choice haunt event-time processing later? What is the partition key for telemetry and why? How many partitions, and how hard is it to change later? What does the projector do when Postgres is down — block, buffer, or drop?

**Reflection questions.** What did setup cost vs. features? Which "temporary" choice (e.g., Postgres-as-truth) will be painful to undo? Is your tick deterministic enough that a replay would reproduce state exactly?

---

## Sprint 2 (Weeks 3–4) — Event Sourcing, CQRS, Idempotency & Replay

**Objective.** Make the event log the real source of truth. Add the live read model (Redis + WebSocket), idempotent consumers, schema-registry-enforced events, the outbox for operator commands, and replay tooling that rebuilds a read model from scratch.

**User/system-visible outcome.** The world view updates *live* (pushed, not polled); you can wipe Postgres and replay the log to rebuild identical state; you can issue an operator command (spawn/disease) from the console.

**Functional requirements.** Engine append-then-publish with stable event ids + per-entity sequence; operator commands via API→outbox→engine; Redis live snapshot + pub/sub fan-out; consumers dedup; a CLI replay command.

**Infrastructure.** Add Redis to Compose; enable schema registry; add a DLQ topic per consumer.

**Backend.** Idempotency utilities (dedup sets in Redis); outbox table + relay; WebSocket/SSE endpoint fed by Redis pub/sub; replay CLI in `services/tooling`.

**Data-platform.** Schema definitions in `contracts/schemas` registered with compatibility mode; second read model (Redis) derived from the same log; retention sized for "replay window."

**Distributed-systems concepts.** Event sourcing vs state-oriented persistence; CQRS (two read models, one log); idempotent consumers and exactly-once *effect* via dedup; transactional outbox vs dual-write; consumer groups & rebalancing; per-entity ordering; DLQ for poison messages; the commands-aren't-replayed distinction. **Guarantees study thread (reason about + document, don't build):** replication factor, producer `acks`, and the in-sync-replica set — what durability each setting actually buys; what partition-leader failover does to ordering and to in-flight produces; and the precise consistency model each read model offers (Postgres strong-on-current vs Redis last-write-wins vs the eventual convergence of replay). Capture these as ADR notes now; you'll test them under failure in S3.

**Frontend.** Swap polling for live push; add an operator command panel (spawn organism, inject disease).

**Testing.** Integration (testcontainers): outbox→publish→projector→store; dedup under duplicate delivery. Property-based: random valid command sequences preserve invariants. Replay/determinism: wipe + replay ⇒ identical Postgres + Redis. Contract: schema-compat check in CI.

**Observability.** Consumer-lag panel; DLQ count/age panel; trace context propagated through Redpanda headers (the showpiece trace: command→engine→topic→projector).

**Failure scenarios.** Duplicate-deliver an event (assert dedup); send a malformed event (assert it lands in DLQ, not a crash); kill a consumer mid-stream and confirm group rebalancing + no loss; corrupt the Postgres read model and rebuild via replay.

**ADRs.** #6 delivery semantics, #7 event sourcing & CQRS, #8 outbox, #16 schema evolution (mode chosen).

**Definition of done.** Live UI; a recorded replay that reproduces byte-identical read models; DLQ + lag visible in Grafana; a working command path with an audit record.

**Deliverable/demo.** "Watch me delete the database and rebuild it from events" — the canonical event-sourcing demo.

**Tech debt deferred.** No Flink yet (no windowed/stateful processing); anomaly detection absent; no lakehouse; auth is stubbed.

**Risks & scope reductions.** *Risk:* outbox + idempotency + replay is a lot for one sprint. *Reductions:* implement the outbox for commands only (engine events can append-then-publish without a separate outbox); replay only the Postgres model this sprint (Redis next); single schema evolution deferred to S4.

**Content ideas.**
- *"Delete your database on purpose: event sourcing explained."* Lesson: when the log is truth, stores are disposable. Demo: live wipe + replay to identical state. Problem: people conflate the database with the truth. Valuable because the demo makes an abstract pattern visceral.
- *"Idempotent consumers: why 'at-least-once' is fine actually."* Lesson: dedup turns redelivery from a bug into a non-event. Demo: force duplicates, show counts unchanged. Problem: exactly-once delivery is a myth; exactly-once *effect* is the real goal. Valuable: corrects a common misconception with a live proof.
- *Short:* "Transactional outbox in 60 seconds" — the dual-write problem animated.

**Learning resources.** DDIA ch.11 (stream processing) + **ch.5 (replication: leaders/followers, sync vs async, failover)** for the guarantees thread; Fowler's *Event Sourcing* and *CQRS* articles; Ben Stopford *Designing Event-Driven Systems* (the event-sourcing/CQRS chapters); Confluent Schema Registry compatibility docs; the Kafka/Redpanda docs on `acks`, replication, and ISR.

**Implementation questions.** What exactly is your idempotency key, and is it stable across replays? Is your read-model reducer order-tolerant, or does it assume strict ordering you don't actually guarantee cross-partition? What must never be replayed? When two consumers in a group rebalance, what's the worst interleaving and does your dedup survive it?

**Reflection questions.** Which guarantees are real vs assumed? After a failure, what's actually recoverable and what's lost? Which component now has unclear ownership? What would break at 10× event volume?

---

## Sprint 3 (Weeks 5–6) — Flink (Java DataStream): Stateful Processing, Anomaly Detection & Alerting

**Objective.** The deep stream-processing sprint. A Java Flink job consumes telemetry + events, keeps keyed state, computes event-time windowed aggregates with watermarks, detects anomalies (vital anomalies, population crashes, telemetry gaps = "device offline"), runs CEP for disease/predation cascades, and emits alerts that reach the console — all with exactly-once checkpointing.

**User/system-visible outcome.** Inject a disturbance (drought, disease, silence an organism) and watch an alert appear in the console within seconds; kill the Flink job and watch it recover from a checkpoint with no double-counting.

**Functional requirements.** Keyed state per organism/region; tumbling/sliding event-time windows; watermark strategy for out-of-order/late data; anomaly rules + CEP; alerts to an alert topic → projector → Redis → console; checkpointing on.

**Infrastructure.** Add Flink (JobManager/TaskManager) to Compose; alert topic; checkpoint storage (local FS now, S3 later).

**Backend.** Java DataStream job(s); alert projector (Go) into Redis/Postgres alert history; the "silence telemetry" operator command to drive the gap demo.

**Data-platform.** Windowed aggregates as a streaming read input; alert history persisted; baselines for anomaly thresholds (simple statistical baseline first).

**Distributed-systems concepts.** Event time vs processing time; watermarks; windowing; keyed state & state backends; checkpoints/barriers and exactly-once; late-data handling (allowed lateness, side outputs); backpressure; CEP; the gap-detection-as-liveness idea.

**Frontend.** Alert console (live list, severity, entity, window); a visual "offline" indicator on silenced organisms.

**Testing.** Flink test harness/MiniCluster for operators; determinism: replay a fixed event log ⇒ identical alert set; late-data test (inject out-of-order, assert correct windowing); checkpoint-recovery test. **First load baseline (front-loaded from S6, per your DS priority):** drive elevated telemetry volume and record the initial throughput ceiling, watermark lag, and backpressure behavior — establish the numbers now so scaling work isn't trapped entirely in the buffer sprint.

**Observability.** Flink metrics (checkpoint duration/size, restarts, backpressure, watermark lag) into Prometheus/Grafana; alert-latency SLI (anomaly→console).

**Failure scenarios.** Kill the TaskManager mid-window (assert recovery, no double alerts); flood out-of-order/late telemetry (assert correct windows + side-output for too-late); induce backpressure (slow sink) and observe it propagate. **Guarantees thread test:** force a partition-leader failover in Redpanda (kill the broker holding a leader) and observe the real effect on ordering, in-flight produces, and consumer continuity — then compare it against what your S2 ADR predicted.

**ADRs.** #6 (delivery semantics finalized for the Flink path), #18 anomaly strategy, plus a state-backend/windowing decision recorded under architecture.

**Definition of done.** Injected disturbance ⇒ alert within budget; checkpoint recovery demonstrated; replay yields identical alerts; Flink dashboard live.

**Deliverable/demo.** "I made a satellite go dark and the system caught it in 3 seconds" — telemetry-gap detection, framed explicitly toward real-time device monitoring.

**Tech debt deferred.** No lakehouse sink yet (alerts/aggregates are in-memory/Redis only); baselines are crude; full scaling/parallelism *tuning* is deferred to S6 — but the initial load baseline and backpressure numbers are captured here.

**Risks & scope reductions.** *Risk:* Flink + Java + event-time is the steepest learning curve in the project; this can overrun. *Reductions:* start with one anomaly rule (telemetry gap) before vitals/population; defer CEP to S6 if needed; use processing time first, then refactor to event time once the pipeline works (and film the refactor — it's great content).

**Content ideas.**
- *"Event time vs processing time: the bug that hides until it doesn't."* Lesson: why your "real-time" averages are wrong under out-of-order data. Demo: same data, two results; fix with watermarks. Problem: the single most misunderstood streaming concept. Valuable because you show the wrong answer first.
- *"Exactly-once isn't magic — it's checkpoints."* Lesson: how Flink recovers without double-counting. Demo: kill the job mid-window, show counts stay correct. Problem: people cargo-cult "exactly-once." Valuable: demystifies with a live kill.
- *"Detecting a device that went silent (with a fish)."* Lesson: absence-of-signal is harder than presence. Demo: silence an organism, watch the gap alert. Problem: gap detection needs liveness reasoning, not thresholds. Valuable: directly maps to telemetry/IoT/satellite monitoring — your target domain.

**Learning resources.** Flink docs: DataStream, event time & watermarks, working with state, checkpointing. *Streaming Systems* (Akidau et al.) ch.1–3 (the definitive event-time treatment). One Flink Forward talk on state/checkpoints. Revisit DDIA ch.11 windows.

**Implementation questions.** What's your watermark strategy and bounded-lateness budget, and what happens to events beyond it? What's keyed-state growth over time and when do you need TTL? Which results must be exactly-once and which can be at-least-once? How do you make alert rules deterministic so replays reproduce them?

**Reflection questions.** What became more complex than expected (almost certainly event-time)? Which guarantee is real vs assumed in your pipeline? What would be hardest to recover after a Flink failure? What would break at 10× telemetry volume — state size, checkpoint duration, or backpressure?

---

## Sprint 4 (Weeks 7–8) — The Data Plane as Infrastructure: Exactly-Once Sink, Iceberg Maintenance, Data-Quality & Freshness SLOs

> **Emphasis (per your priority).** This sprint is *data-platform infrastructure*, not analytics engineering. The depth goes into the exactly-once stream→table sink, Iceberg maintenance as an operational concern, the schema registry as a platform contract, and data-quality + freshness as monitored *platform SLOs*. dbt and marts are deliberately minimal here — just enough to prove the ELT seam exists — and the heavy analytical modeling stays out of scope on purpose.

**Objective.** Build the ingestion-and-storage plane that other people's analytics *would* run on. Flink writes raw telemetry + events to Iceberg exactly-once; Dagster operates the table maintenance (compaction, snapshot expiry, partition-spec evolution) and runs data-quality + freshness gates that behave like platform SLOs; the schema registry enforces contracts across a live consumer set; lineage is visible end to end. A single minimal dbt mart demonstrates the ELT seam and then you stop.

**User/system-visible outcome.** Telemetry lands in Iceberg exactly-once and is queryable within a freshness budget you can show on a dashboard; injecting bad/late data trips a quality or freshness gate that halts the affected pipeline (not the world); a Dagster lineage graph shows provenance from topic to table. One demonstration query proves the path is usable — the query is evidence the *platform* works, not the deliverable itself.

**Functional requirements.** Flink Iceberg sink (raw layers, exactly-once); Dagster assets for compaction/snapshot-expiry/partition-evolution and for DQ + freshness gates; an operational backfill/recompute capability; a bad-data quarantine path; one deliberate schema evolution exercised end-to-end against live consumers; *one* minimal dbt mart (no more) to show the ELT seam.

**Infrastructure.** MinIO locally / S3 + Glue catalog + Athena workgroup in cloud (Terraform first pass, even if you mostly run local). Dagster + dbt in Compose.

**Backend.** Flink sink configuration to Iceberg; the backfill/recompute job; a minimal API endpoint to run the one demonstration query (keep the frontend thin).

**Data-platform.** Iceberg table layout as an *operational* decision (partitioning/sort + snapshot/expiry + compaction cadence and their write-amplification/read-latency tradeoffs); the maintenance jobs that keep the lake healthy under continuous writes; DQ checks treated as platform SLOs (freshness, completeness/gaps, referential "every died has a born", distribution, invariants) with alerting; Dagster asset lineage; the ELT seam (land raw, *one* transform); operational recompute over a time-travel snapshot; a quarantine path for non-conforming partitions.

**Distributed-systems concepts.** Exactly-once stream→table sink mechanics and how Iceberg snapshot isolation makes concurrent Flink-write/Athena-read safe; the stream-vs-batch boundary as an ownership decision; schema evolution propagated across a live consumer set without breakage; operational replay vs analytical recompute.

**Frontend.** Intentionally minimal: surface freshness/DQ status and the single demonstration query result in the existing console or in Grafana. No analytics dashboard build-out — that's the analytics-engineering work you're deprioritizing.

**Testing.** Schema-compatibility test (push the deliberate evolution, prove old + new consumers coexist); Dagster DQ/freshness gates demonstrably halt downstream on failure; determinism test on a recompute (same snapshot ⇒ same output); a single dbt test on the one mart to show the seam works. No broad mart test suite.

**Observability.** Pipeline-freshness SLI (event-time → queryable in Athena) with an error budget; DQ results as time series with alerts; Iceberg snapshot/compaction/file-count metrics (small-files problem made visible); Dagster run status surfaced in Grafana.

**Failure scenarios.** Late-arriving partition (assert freshness alert + correct backfill); schema drift from a non-compliant producer (assert registry/DQ catch it + quarantine); compaction running during a read (assert snapshot isolation holds); a failed maintenance/gate job (assert Dagster halts dependents, not the world); small-files accumulation degrading reads (assert compaction recovers it).

**ADRs.** #10 Iceberg table design + maintenance policy, #11 stream-vs-batch boundary, #12 orchestration, #16 schema evolution (the staged breaking change).

**Definition of done.** Telemetry lands exactly-once and is queryable within the freshness budget; DQ/freshness gates demonstrably block bad data and surface on a dashboard; compaction keeps the table healthy under sustained writes; the schema evolution shipped without breaking consumers; one minimal mart proves the ELT seam.

**Deliverable/demo.** "An exactly-once telemetry ingestion platform: from emission to queryable table, with quality and freshness enforced." Show the freshness dashboard, trip a DQ gate live, and run the one query to prove the path — the *platform*, not the chart, is the point.

**Tech debt deferred.** Still on Docker Compose (no k8s); auth still stubbed; analytical marts intentionally left at one (build more later only if a real analytics need appears); no load testing of the sink yet (baseline comes from S3, full load in S6).

**Risks & scope reductions.** *Risk:* the Flink→Iceberg sink + Athena/Glue setup has fiddly integration points. *Reductions:* land raw exactly-once first and treat maintenance/DQ as the depth; use DuckDB locally and only wire real Athena if cloud time allows; one DQ check of each type rather than exhaustive coverage; the dbt mart can slip to "stub" if the sink + maintenance eat the sprint.

**Content ideas.**
- *"Exactly-once from a stream into a table (the hard part nobody shows)."* Lesson: how Flink + Iceberg coordinate commits so a crash mid-write doesn't duplicate or lose rows. Demo: kill the sink mid-commit, show the table stays correct. Problem: most "streaming to a lake" tutorials quietly give you at-least-once. Valuable: shows the genuinely hard guarantee, the part that's platform engineering.
- *"Your data lake is rotting: the small-files problem and compaction."* Lesson: why a healthy lake needs operational maintenance, not just writes. Demo: query latency degrading, then compaction fixing it. Problem: lakes are treated as write-and-forget. Valuable: a real ops concern that separates platform engineers from analysts.
- *"Data quality as a platform SLO, not a dbt test."* Lesson: freshness/completeness gates that halt pipelines and page you. Demo: trip a gate, watch the pipeline stop and the alert fire. Problem: quality is usually an afterthought bolted onto marts. Valuable: frames DQ as infrastructure with budgets, which is the platform mindset.

**Learning resources.** Iceberg docs (table spec, partitioning, **maintenance/compaction/snapshot expiry**); Flink Iceberg connector + the two-phase-commit sink docs; Dagster docs (software-defined assets, sensors/schedules); DDIA ch.10 (batch) for the stream/batch framing. dbt docs only enough to wire one model — don't go deep on analytics engineering.

**Implementation questions.** What's your Iceberg partitioning + sort + compaction policy, and what write-amplification/read-latency tradeoff does it embody? How exactly does the Flink→Iceberg sink achieve exactly-once (what's the commit protocol), and what happens on a crash between checkpoint and commit? What is computed in the stream vs in batch, and who *owns* each table? Which compatibility mode do you enforce, and how do you stage a breaking change against live consumers without downtime?

**Reflection questions.** Where did batch and streaming responsibilities blur, and who should own the boundary? What does compaction cost you under sustained load, and what's the small-files breaking point? What would an operational recompute cost at 100× history? Which freshness/DQ SLO would have caught a real bug you already hit — and did you set a budget for it?

---

## Sprint 5 (Weeks 9–10) — Real Deployment: Kubernetes (k3s), IaC, Security & Full Observability

**Objective.** Take the system off your laptop. Stand up k3s on Oracle Cloud always-free ARM, deploy services + Flink + the observability stack via Helm/Kustomize, provision the AWS surface with Terraform, add real authn/authz and secret management, and complete the tracing/SLO story across the cluster. **And solve the coordination problem k8s forces on you:** the simulation engine is the single authoritative writer and event source — so deploying it on an orchestrator that can reschedule, restart, or accidentally double-run it raises split-brain. This sprint makes the engine a *safe* singleton via leader election + fencing.

**User/system-visible outcome.** A public (auth-gated) console and Grafana running on your own cluster; an operator logs in, issues a command, and you follow that one command as a distributed trace across services; SLO dashboards with error budgets; and a demonstrated failover where one engine instance dies, another takes leadership, and the dead instance is *fenced* so it cannot resume writing — with zero duplicate or conflicting events.

**Functional requirements.** All services containerized + deployed to k3s; Flink on k8s; Neon/Upstash as managed Postgres/Redis; AWS S3/Glue/Athena via Terraform; OIDC/JWT auth + RBAC; secrets via SOPS/External Secrets; end-to-end traces; SLOs + alerts; **engine leader election with a fencing token so exactly one instance writes truth at a time and a stale leader is prevented from committing after losing the lease.**

**Infrastructure.** Oracle ARM node(s) + k3s; Helm/Kustomize; Terraform for AWS; an IdP (Keycloak on-cluster or a hosted free tier); Alertmanager.

**Backend.** Auth middleware + RBAC in serving; audit logging of commands; config/secret wiring for cloud endpoints; health/readiness probes; leader election for the engine (lease-based — e.g., a k8s Lease object or a lock in Postgres/Redis) with a monotonic fencing token attached to every write/produce so a fenced stale leader's commits are rejected.

**Data-platform.** Point Flink checkpoints + Iceberg at real S3; Dagster scheduled in-cluster (or kept local with cloud Athena — your call, document it).

**Distributed-systems concepts.** Orchestration, scheduling, rolling deploys, self-healing, network policies; managed-vs-self-operated split; trace-context propagation across the real network and through Redpanda; leader election, leases and lease expiry, fencing tokens, split-brain, and why "just run one replica" is not a safety guarantee on an orchestrator that can run two during a partition or a slow restart.

**Frontend.** Login flow; role-gated controls (operator vs viewer vs analyst); a status/health page.

**Testing.** End-to-end against the deployed cluster (command→world→read-model→UI); auth tests (unauthorized command rejected); a deploy smoke test in CI; trace assertion (one command produces a connected trace).

**Observability.** Full stack live (metrics+logs+traces) in-cluster; SLIs/SLOs defined with error budgets (event latency, alert latency, freshness, API availability); Alertmanager routing; consumer-lag/DLQ/freshness/DQ alerts wired.

**Failure scenarios.** Kill a pod (assert reschedule + recovery); drain the node / simulate node loss; rolling deploy with in-flight traffic (assert no dropped commands beyond guarantees); revoke a secret (assert graceful failure, not silent compromise); break an SLO deliberately and watch the error budget burn. **Split-brain test:** force two engine instances to run concurrently (e.g., scale to 2, or pause the leader so its lease expires while it's still "alive") and prove the fencing token prevents the stale instance from committing — no duplicate or conflicting events reach the log.

**ADRs.** #13 deployment (k3s vs EKS), #14 IaC choices, #15 observability stack + trace propagation, #17 local-vs-cloud split, **#19 engine singleton coordination (leader-election mechanism + fencing strategy)**, plus security boundaries.

**Definition of done.** The system runs on your cluster within budget; a single command is traceable end-to-end; auth/RBAC enforced; SLO dashboard live; a runbook exists for at least pod loss and SLO breach; **a recorded failover shows leadership transfer with the old leader fenced and zero duplicate/conflicting events.**

**Deliverable/demo.** "One click, one trace: following an operator command across six hops in production," plus **"Killing the brain: how one instance safely takes over the world without split-brain"** (the leader-election + fencing failover). Include a short cost breakdown proving ≤ $50/month.

**Tech debt deferred.** mTLS/service mesh; multi-node HA; autoscaling tuning (S6); exhaustive runbooks.

**Risks & scope reductions.** *Risk:* k8s + IaC + auth in one sprint is the second-steepest climb. *Reductions:* single-node k3s first (HA later); keep Dagster local against cloud Athena if in-cluster scheduling overruns; use a hosted free IdP instead of self-hosting Keycloak; Kustomize over Helm if templating becomes a time sink.

**Content ideas.**
- *"I deployed a real streaming platform for $0 (here's the catch)."* Lesson: free-tier architecture and its honest limits. Demo: the running cluster + the bill. Problem: "cloud-native" usually means "expensive." Valuable: a reproducible blueprint others can copy.
- *"Tracing one request across six async hops."* Lesson: context propagation through Kafka/Redpanda headers. Demo: the trace waterfall for one command. Problem: async systems are invisible without distributed tracing. Valuable: shows the single most useful debugging tool for event systems.
- *"SLOs and error budgets for a side project (yes, really)."* Lesson: define reliability targets and watch them burn. Demo: break an SLO live. Problem: reliability without targets is vibes. Valuable: brings an SRE practice down to a buildable scale.
- *"My system has one brain — here's how I stop two from running."* Lesson: leader election + fencing tokens, and why a single replica isn't safety. Demo: force two engines, show the fenced one rejected, zero double-writes. Problem: stateful singletons on k8s are a classic split-brain trap people don't see coming. Valuable: this is exactly the distributed-coordination depth that separates "I used Kafka" from "I understand distributed systems" — and it's directly interview-relevant for your target roles.

**Learning resources.** k3s docs + Kubernetes "concepts" basics; *Kubernetes Up & Running* (selected chapters: pods, deployments, services, config/secrets); Google SRE book chapters on SLIs/SLOs and monitoring; Terraform AWS provider docs (S3/Glue/Athena/IAM); OTel tracing + propagation docs.

**Implementation questions.** What's your managed-vs-self-operated rule, and why does Postgres/Redis sit on the managed side of it? How do secrets get into pods without touching git? What's your minimum viable RBAC model? Which SLOs matter to a *user* vs to *you on-call*? How does trace context survive the Redpanda hop? **What is your engine's leader-election mechanism, how long is the lease, and what happens in the window between a leader stalling and its lease expiring — can a fenced leader still commit one last write, and how do you make the producer/sink reject it?**

**Reflection questions.** What would need to change for a real production deployment (HA, mTLS, scaling)? Which operation is still not idempotent under k8s rescheduling? What would be hardest to recover if the node died right now? Where did "works locally" not equal "works in-cluster," and why?

---

## Sprint 6 (Weeks 11–12) — Scale, Harden, Chaos & Portfolio Packaging (and buffer)

**Objective.** Push the system until it bends, fix what breaks, run deliberate chaos, optionally add the high-write NoSQL telemetry store, and package everything into a portfolio + content artifact. This sprint also absorbs slip from Sprints 3–5.

**User/system-visible outcome.** A load demo showing the system sustaining high telemetry volume with bounded lag; a chaos exercise with a written postmortem; a polished README + architecture docs + demo video.

**Functional requirements.** Load generator drives high volume; consumers/Flink scale horizontally; backpressure stays bounded; chaos experiments pass; (optional) a recent-raw-telemetry query served from a NoSQL hot store; finalized docs.

**Infrastructure.** Scale TaskManagers/consumer replicas; (optional) Scylla/ClickHouse; chaos tooling (scripts or a lightweight chaos operator).

**Backend.** Tune consumer concurrency + Flink parallelism; (optional) a hot-store projector; performance fixes surfaced by load tests.

**Data-platform.** Validate Iceberg compaction under sustained writes; finalize remaining marts deferred from S4; alert precision/recall analysis over a deterministic replay.

**Distributed-systems concepts.** Horizontal scaling, partition rebalancing under load, backpressure limits, chaos engineering, capacity reasoning, and the hot-vs-cold storage tradeoff (if NoSQL added).

**Frontend.** Polish; a "system health/scale" view; finalize the analytics view.

**Testing.** Load tests (throughput ceiling, p99 latency, lag under load); chaos/failure-injection as a suite (broker loss, node loss, slow sink, schema violation); the full replay/determinism suite as a regression gate.

**Observability.** Saturation dashboards; capacity headroom; alert-quality metrics (precision/recall from replays); finalize runbooks for every failure class exercised.

**Failure scenarios.** Sustained overload (assert graceful degradation, not collapse); drop a broker under load; scale a consumer group mid-stream (assert rebalancing without loss); the full chaos battery run as a recorded exercise.

**ADRs.** #18 anomaly strategy (revisited with precision/recall data); a scaling/capacity decision; (optional) the NoSQL hot-store decision (#9 revisited).

**Definition of done.** A documented throughput ceiling with bounded lag; a chaos postmortem in `docs/runbooks`; README + diagrams + ADRs complete; a 3–5 min demo video; the repo is something you'd put at the top of a resume.

**Deliverable/demo.** The capstone video: architecture tour → live world → injected outbreak → real-time alert → lakehouse query → trace → chaos kill + recovery → the cost slide.

**Tech debt deferred (knowingly, and documented as "future work").** Multi-region partitioning; ML-based anomaly baselines; service-mesh mTLS; multi-node HA; materialized-view read models. Listing these *deliberately* is itself a senior signal.

**Risks & scope reductions.** *Risk:* this sprint carries slip from S3–S5. *Reductions (protecting your DS priority):* the scaling/backpressure work and at least the top three chaos experiments are *protected* — they are the point of this sprint. What shrinks first instead is the optional NoSQL hot store (drop entirely), the remaining analytical marts (leave at one), and UI polish. Load-test the streaming path before the batch path if time is tight.

**Content ideas.**
- *"I tried to break my own system on purpose."* Lesson: chaos engineering finds what tests don't. Demo: kill a broker under load, narrate recovery. Problem: you don't know your failure modes until you cause them. Valuable: the postmortem is the content — including what surprised you.
- *"Where it breaks at 10×: a capacity story."* Lesson: find the real bottleneck (state size? lag? compaction?). Demo: the throughput-vs-lag curve. Problem: "it scales" is meaningless without a number. Valuable: honest limits beat fake confidence.
- *"3 months of building a distributed system: what I'd do differently."* Lesson: the retrospective. Demo: the architecture evolution montage. Problem: reflection is where learning compounds. Valuable: the series-capstone that converts viewers into followers and reviewers into believers.

**Learning resources.** *Database Internals* (Petrov) — selected chapters on storage/replication for the hot-store decision; *Principles of Chaos Engineering* (the manifesto + one practitioner talk); k6 or Locust docs; Flink scaling/parallelism docs; revisit DDIA ch.6 (partitioning) with real load in hand.

**Implementation questions.** Where is the actual bottleneck under load, and how do you know (which metric)? Does adding a NoSQL hot store earn its operational cost, or is Athena-on-Iceberg enough? What's your partition count's headroom for scaling consumers? Which chaos experiment taught you something a unit test never could?

**Reflection questions.** What assumptions failed across the whole project? What would break at 10× and at 100×? Which guarantees turned out to be assumed rather than real? What would be genuinely hard to recover in production? Which three things would you build differently from sprint one — and which became your best educational content?

---

# PART C — CROSS-CUTTING NOTES

**Content cadence (to confirm).** A natural rhythm given two-week sprints: one long-form build/explainer video per sprint (the "deliverable/demo" is the spine), 1–2 shorts mid-sprint (a single concept animated), and a written sprint retrospective. That's ~6 long-form + ~10 shorts + 6 retros over the project — enough to build momentum without letting filming eat building time. If filming threatens the 15-hour budget, the long-form videos are the priority; shorts are the cut.

**The through-line for your target roles.** Every sprint deliberately produces a signal a distributed-systems/backend and data-platform interviewer cares about, in your stated priority order: event sourcing + idempotency + replay + the guarantees thread (S2), exactly-once stateful streaming + anomaly/gap detection + a load baseline (S3), an exactly-once ingestion platform with Iceberg maintenance and DQ/freshness SLOs — infrastructure, not analytics (S4), real k8s + IaC + leader-election/fencing + distributed tracing + SLOs (S5), load + scaling + chaos + honest capacity limits (S6). The telemetry framing (organisms as devices, gap = offline, region rollups = fleet health) is the bridge to the Starlink-style work you're aiming at — keep that analogy explicit in the README and the capstone video without letting ecological realism or analytics dashboards become the point.

**Sequencing safety valve (ordered to your priorities: DS/backend first, data-platform infra second, analytics last).** If you fall behind, protect the spine in this order. *Never sacrifice the distributed-systems core:* keep S1–S3 whole (event sourcing, idempotency, replay, exactly-once streaming, the guarantees thread), keep S5's leader-election/fencing and tracing, and keep S6's load + scaling + chaos + capacity work — this is your #1 signal and must not become the buffer that disappears. *Shrink the data-platform infra next, but keep its core:* in S4, protect the exactly-once sink + Iceberg maintenance + DQ/freshness gates; the operational recompute can slip. *Cut analytics and polish first:* the single dbt mart can drop to a stub, the analytics view can stay in Grafana, the optional NoSQL hot store goes, and UI polish is last. A finished system that is deep on distributed systems and honest about its limits beats a broad one — and it demos far better on camera.
