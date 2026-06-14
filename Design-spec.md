# LOAR

## Design Specification v0.2

---

# Status

This document defines the current design direction for Loar.

It is intentionally incomplete.

Loar is being developed through experimentation across multiple domains and projects. The concepts described here are hypotheses that must be validated through usage.

The purpose of this document is to provide a clear implementation direction while preserving flexibility.

When evidence contradicts assumptions in this document, the design should change.

---

# What Is Loar?

Loar is a knowledge runtime.

It ingests information, structures knowledge, preserves relationships, and retrieves evidence.

Loar exists to improve decision making by making relevant knowledge available when needed.

Loar is infrastructure.

It is not an AI assistant.

It is not a chatbot.

It is not a vector database.

It is not a graph database.

It is not a memory wrapper.

Those technologies may exist beneath Loar.

Loar itself is the system responsible for transforming information into retrievable knowledge.

---

# Philosophy

Most systems focus on storing data.

Loar focuses on retrieving understanding.

Traditional systems answer:

```text
What happened?
```

Loar aims to answer:

```text
What happened?

Why?

What evidence supports this?

What changed?

What do we know now?

What should influence our decision?
```

---

# Primary Objectives

Loar must:

* Work across any domain
* Work across multiple projects
* Operate locally
* Support humans and agents equally
* Function without an LLM
* Generate context for LLMs when needed
* Prioritise evidence over opinion
* Preserve temporal meaning

---

# Architecture Overview

## Data Flow

```text
Source Data
    ↓
Ingestion
    ↓
Normalisation
    ↓
Knowledge Store
    ↓
Retrieval Engine
    ↓
Context Package
    ↓
Human / Agent / LLM
```

## Knowledge Pipeline

Within the knowledge store and retrieval engine, information is structured
and reasoned about through the following stages. None of these stages require
an LLM.

```text
Observations
    ↓
Entities  (pattern extraction, alias matching, n-gram resolution)
    ↓
Relationships  (co_occurs, mentions — derived deterministically at ingestion)
    ↓
Temporal Reasoning  (occurred_at, observed_at, resolved_at, learned_at)
    ↓
Evidence Ranking  (recency-weighted confidence, contradiction detection)
    ↓
Context Package
```

---

# Core Design Principles

## Retrieval First

Retrieval is the core product.

LLMs are consumers of Loar.

Bad:

Question
→ LLM
→ Answer

Good:

Question
→ Loar
→ Evidence
→ Optional LLM

---

## Knowledge Before AI

The source of truth belongs to Loar.

The LLM is a presentation layer.

Not a storage layer.

---

## Universal Design

Loar must support:

* football transfers
* software delivery
* stock market analysis
* simulations
* future unknown domains

Domain-specific concepts belong in adapters.

Not the kernel.

---

## Project Isolation

Knowledge is scoped.

Projects should not automatically contaminate one another.

A project acts as a knowledge boundary.

---

## Human And Agent Equality

Humans and agents use the same system.

Agents should not require hidden APIs.

Everything should be accessible through Loar.

---

# CLI Design

The CLI is the primary interface.

Consumers include:

* humans
* AI agents
* applications
* automation
* workflows

---

## Project Selection

```bash
loar project use tierone
```

Associates current directory with a project.

Creates:

```text
.loar/project.toml
```

Example:

```toml
project = "tierone"
```

---

## Ingestion

```bash
loar ingest transfers.json
```

```bash
cat data.ndjson | loar ingest
```

```bash
loar ingest https://example.com/feed
```

Supports:

* files
* streams
* urls
* batches

---

## Question Interface

```bash
loar "Why is Romano reliable?"
```

```bash
loar "What evidence supports this signal?"
```

```bash
loar "Show unresolved observations about Player X"
```

Users provide questions.

Loar determines retrieval strategy.

---

## Explanation

```bash
loar explain "Why is Romano reliable?"
```

Produces human-readable narrative output based on evidence.

---

# Retrieval Model

```text
Question
↓
Intent Detection  (causal, timeline, comparison, evidence, general)
↓
Entity Resolution  (n-gram matching, alias lookup, ILIKE fallback)
↓
Relationship Traversal  (co_occurs graph from entity store)
↓
Evidence Gathering  (entity-scoped first, keyword fallback)
↓
Context Package
```

Not:

```text
Question
↓
Embedding
↓
Vector Search
↓
Answer
```

Vector search may be added later as a complementary signal.

It is not the primary retrieval model and is not required for correctness.

Intent shapes the retrieval path:

* `timeline` — observations sorted chronologically by `occurred_at`
* `causal` / `evidence` — contradiction cap raised; tensions surface more aggressively
* `comparison` — entity resolution returns multiple entities; observations merged across all

---

# Core Domain Model

Current candidate kernel primitives:

---

## Observation

The smallest meaningful unit.

Examples:

* rumour
* claim
* prediction
* event
* outcome
* signal

Everything begins as an Observation.

---

### Observation Structure

```go
type Observation struct {
    ID string

    ProjectID string

    Content string

    SourceID string

    Temporal Temporal

    Metadata map[string]any
}
```

---

## Entity

Something that exists.

Examples:

* person
* company
* team
* stock
* project
* organisation
* character

Entities act as retrieval anchors.

---

### Entity Structure

```go
type Entity struct {
    ID string

    Type string

    CanonicalName string

    Aliases []string

    Metadata map[string]any
}
```

---

## Relationship

Connects things.

Examples:

```text
reported_by
caused
supports
contradicts
related_to
part_of
```

---

### Relationship Structure

```go
type Relationship struct {
    ID string

    SourceID string

    TargetID string

    Type string

    Confidence float64
}
```

---

## Project

Knowledge boundary.

---

### Project Structure

```go
type Project struct {
    ID string

    Name string

    Description string
}
```

---

# Temporal Model

Time is a first-class concern.

Single timestamps are insufficient.

---

### Temporal Structure

```go
type Temporal struct {
    OccurredAt *time.Time
    ObservedAt *time.Time
    ResolvedAt *time.Time
    LearnedAt  *time.Time
}
```

Definitions:

OccurredAt

When the event happened.

ObservedAt

When Loar became aware.

ResolvedAt

When outcome became known.

LearnedAt

When Loar derived new understanding.

---

# Storage Design

Current recommendation:

```text
Postgres
+
MinIO
```

---

## Postgres

Stores:

* projects
* observations
* entities
* relationships
* sources
* retrieval metadata

---

## MinIO

Stores:

* PDFs
* CSVs
* articles
* exports
* raw assets

Loar stores references.

Not large payloads.

---

# Database Tables

## projects

```sql
id
name
description
created_at
```

---

## entities

```sql
id
project_id
type
canonical_name
aliases
metadata
created_at
```

---

## observations

```sql
id
project_id
source_id
content
occurred_at
observed_at
resolved_at
learned_at
metadata
created_at
```

---

## relationships

```sql
id
project_id
source_id
target_id
relationship_type
confidence
created_at
```

---

## observation_entities

```sql
observation_id
entity_id
role
```

---

# Retrieval Output

Loar should never expose raw database rows as its primary output.

It should generate a Context Package.

---

## Context Package

```go
type ContextPackage struct {
    Query string

    Summary string

    Entities []Entity

    Observations []Observation

    Relationships []Relationship

    Timeline []TimelineEvent

    Evidence []Evidence

    Contradictions []Contradiction

    Confidence float64

    RelatedTopics []string
}
```

---

# LLM Role

90–95% of Loar is implemented without an LLM. The most valuable parts
do not depend on one.

| Component | Needs LLM? |
|---|---|
| Observation storage | No |
| Entity resolution | No |
| Project isolation | No |
| Temporal reasoning | No |
| Relationship derivation | No |
| Evidence ranking | No |
| Confidence calculation | No |
| Retrieval | No |
| Context package generation | No |
| Narrative output | Optional |
| Unstructured ingestion | Helpful |
| Relationship discovery | Helpful |

Where LLMs add genuine value:

1. **Ingestion of unstructured content** — extracting entities, claims, and
   events from long articles, PDFs, and transcripts. Before LLMs this required
   spaCy, custom NER pipelines, and regex. The return on complexity here is
   highest.

2. **Relationship discovery** — surfacing hidden connections between
   observations that share implicit meaning. Not required, but useful.

3. **Narrative generation** — converting a context package into prose for
   human consumption. The `loar query` command does this via a deterministic
   Node.js renderer. An LLM can replace or augment this layer without touching
   the retrieval core.

Loar's moat is not text generation. It is whether a query can produce:

```text
432 observations analysed
89 resolved outcomes
91% accuracy rate
17 independent confirmations
4 contradictions
confidence: 0.87
```

without asking a model anything.

---

# Evidence Ranking

Evidence is ranked without an LLM using two signals:

## Recency

Each observation receives a confidence score based on age:

```
confidence = 0.5 + 0.5 × e^(−days/365)
```

* Observations from today score 1.0
* Observations one year old score ~0.82
* Observations with no temporal data score 0.5 (neutral)

`OccurredAt` is preferred over `ObservedAt` when both are present.

## Source Reliability

Not yet implemented. Planned: sources accumulate a reliability score as
their predictions resolve. A source with a high outcome accuracy rate
receives a multiplier on its observation confidence scores.

---

# Entity Resolution

Entity resolution is currently considered a critical subsystem.

Examples:

```text
Romano

Fabrizio Romano

@FabrizioRomano
```

must resolve to:

```text
Entity: Fabrizio Romano
```

Potential future approaches:

* exact matching
* alias matching
* fuzzy matching
* search index
* semantic matching

Implementation remains open.

---

# Query Intent Detection

Questions should be classified internally.

Examples:

```text
Why is Romano reliable?
```

→ causal analysis

```text
When did Arsenal first show interest?
```

→ timeline

```text
Compare Romano and Ornstein.
```

→ comparison

This classification should remain internal.

Users should not choose query modes.

---

# Future Exploration

Resolved:

* **How should relationships be derived?** — Deterministically at ingestion
  time. Entities co-extracted from the same observation receive a `co_occurs`
  relationship with confidence 1.0. No LLM required.

* **How should entity resolution work?** — Pattern extraction (regex) +
  n-gram matching (trigrams → bigrams → tokens) + ILIKE alias lookup.
  Sufficient for structured knowledge domains. Fuzzy matching is a
  future enhancement.

* **Should intent influence retrieval?** — Yes. Timeline queries sort
  chronologically. Causal/evidence queries surface more contradictions.

Open:

* Is Observation the correct primitive for all domains?
* How should source reliability be tracked and updated as outcomes resolve?
* How should confidence aggregate across a chain of relationships?
* Is co_occurs sufficient or should typed relationships (reported_by, caused,
  supports) be derived from content structure?
* How should cross-project retrieval work?
* How should reflection and learning operate (LearnedAt, ResolvedAt)?
* When should vector search be introduced as a complementary signal?

---

# Success Criteria

Loar is successful if it can:

1. Ingest information from any project.
2. Resolve entities reliably using pattern extraction and alias matching.
3. Build an entity co-occurrence graph at ingestion time without an LLM.
4. Rank evidence by recency and (future) source reliability without an LLM.
5. Retrieve evidence without an LLM.
6. Surface contradictions using deterministic rules.
7. Produce useful context packages.
8. Support both humans and agents through the same interface.
9. Preserve knowledge over time with full temporal structure.
10. Scale across multiple independent projects.

If these goals are achieved, the architecture is moving in the right direction.

If not, the architecture should change.

Reality takes priority over design.
