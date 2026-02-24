# Linear Instance Summary (via MDEMG)

*Generated: 2026-01-23*
*Data Source: MDEMG memory graph (1,948 items synced from Linear)*

## Organization Structure

**6 Teams, 1,948 Total Items:**

| Team | Full Name | Issues | Focus |
|------|-----------|--------|-------|
| TEC | Technology | 1,548 | Core technology, WMS, infrastructure |
| ENG | Engineering | 143 | Process control, equipment issues |
| WHI | Whiskey House E&T | 107 | E&T-specific applications, lab data |
| ETL | Leadership | 8 | Strategic initiatives, DORA metrics |
| ATLAS | ATLAS | 0 | (Placeholder/planning team) |
| WHCAP | Whiskey House CapEx | 0 | (Capital expenditure tracking) |

**136 Projects** across multiple domains including a clear **6-phase roadmap** (P00-P05).

---

## Major Work Streams Identified

### 1. Warehouse Management System (WMS) - 17 projects

- Scanner Command Center, 3D Warehouse Viewer
- Barrel tracking, ownership management
- Job synchronization, inventory checks
- Label printing integration

**Related Projects:**

- 3d/2d Warehouse Viewer for faster navigation
- Add Aggregate Reporting to WMS Based on Pivot Tables
- Advanced Barrel Ownership Management System
- Backend API for Warehouse Job Progress Synchronization
- Barrel & Proof Production Order Integration & Raw Material Selection
- Barrel Sense Warehousing Data Integration
- Barreling / Lot Creation Transaction Automation
- Blend, Cistern, and Barreling Traceability & Batching
- Large Barrel Filler Automation Updates and Improvements
- On Prem Barrel Printer Integration
- WMS - Scanner Command Center Config
- WMS Frontend User Testing & E2E Test Guide
- WMS Phase I Closeout & Cutover
- WMS Scanner Roadmap - Final Release Draft
- Warehouse Job Sync Improvements
- Warehouse Jobs - Inventory Check
- Weekly Barrel Action Summary UI

### 2. Platform Modernization Roadmap (P00-P05)

| Phase | Name | Focus |
|-------|------|-------|
| P00 | Foundation Setup | Infrastructure baseline |
| P01 | Modular Platform Architecture | Service decomposition |
| P02 | SDLC & DevEx Standardization | Developer experience |
| P03 | Data Pipeline & Curated Datasets | Data infrastructure |
| P04 | MLOps + SME Supervision | AI/ML operations |
| P05 | Reliability / Observability / Security | Production hardening |

### 3. Security Initiatives - 5 projects

- Row-Level Security (RLS) Implementation
- DevSecOps Implementation
- Cloud configuration Cybersec
- Networking Penetration Cybersec
- P05 - Reliability / Observability / Security

### 4. AI/ML Initiatives - 3 projects

- Prod AI Chatbot for Customer Portal
- P04 - MLOps + SME Supervision
- Chatbot / Agent Configuration

### 5. Barrel Operations

- Barreling automation, traceability
- Proof production, raw material selection
- Barrel Sense integration
- Large Barrel Filler automation

---

## Observations from MDEMG Consolidation

The hidden layer analysis identified **36 pattern clusters** with 2 meta-concepts:

- **Largest cluster (135 items)**: WMS roadmap items, dashboarding, barrel operations
- **Cross-cutting themes**: Transfer In/Out workflows, job status clarity, label printing
- Teams and projects are highly interconnected through shared terminology

### Top Semantic Clusters

| Cluster | Size | Key Themes |
|---------|------|------------|
| Hidden-linear-issues-7 | 135 | WMS roadmap, dashboarding, barrel ops |
| Hidden-linear-projects-13 | 131 | Cross-project coordination |
| Hidden-mixed-14 | 130 | Mixed issues/projects |
| Hidden-linear-issues-1 | 111 | Transfer workflows |
| Hidden-linear-issues-0 | 108 | Job status, progress tracking |

---

## Recommendations for Optimized Work

### 1. Workload Distribution

TEC carries **85% of issues** (1,548/1,806). Consider:

- Splitting TEC into sub-teams by domain (Infrastructure, WMS, Frontend)
- Using Linear's team hierarchy to maintain visibility while improving focus
- Moving domain-specific issues to WHI/ENG as appropriate

### 2. Project Hygiene

- 136 projects for ~1,800 issues is a high ratio (13 issues/project average)
- Many projects have **empty descriptions** - add context for future reference
- Consider consolidating related projects (e.g., multiple WMS projects into parent/child)

### 3. Roadmap Clarity

- The P00-P05 roadmap structure is excellent for strategic alignment
- Link tactical issues to these strategic projects via parent relationships
- Use milestones within each phase for progress tracking

### 4. Issue Quality

Some issues have vague titles ("Bug caught on localhost" appears multiple times). Standardize issue templates with required fields:

- **What happened / What's expected**
- **Steps to reproduce** (for bugs)
- **Acceptance criteria** (for features)

### 5. Cross-Team Visibility

The MDEMG consolidation reveals natural clusters spanning teams. Consider creating "Epic" or "Initiative" level tracking for:

- **Barrel Operations** (spans TEC, ENG, WHI)
- **Security Hardening** (spans multiple projects)
- **Documentation Migration** (spans all teams)

### 6. Leverage Hidden Patterns

MDEMG identified semantic clusters your team may not see in Linear's flat view. The largest pattern groups suggest these topics deserve dedicated views:

- Transfer workflows & label printing
- Dashboarding roadmap
- Job status/progress tracking

---

## Data Sources

This summary was generated by querying the MDEMG memory graph:

```bash
# Teams
curl -X POST http://localhost:8090/v1/memory/retrieve \
  -d '{"space_id": "linear", "query_text": "team organization", "top_k": 20}'

# Projects
curl -X POST http://localhost:8090/v1/memory/retrieve \
  -d '{"space_id": "linear", "query_text": "project roadmap milestone", "top_k": 50}'

# Issue patterns
docker exec mdemg-neo4j cypher-shell -u neo4j -p testpassword \
  "MATCH (n:MemoryNode) WHERE n.path STARTS WITH 'linear://issues' ..."
```

The hidden layer patterns were created via the consolidation endpoint:

```bash
curl -X POST http://localhost:8090/v1/memory/consolidate \
  -d '{"space_id": "linear"}'
```
