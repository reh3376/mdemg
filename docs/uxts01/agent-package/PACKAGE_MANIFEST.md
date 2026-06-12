# UxXTS Agent Package Manifest

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Generated: 2026-02-28T14:34:51Z

## Inventory

| File | Bytes | SHA-256 |
|---|---:|---|
| `docs/uxts01/agent-package/AGENT_BOOTSTRAP_PROMPT.md` | 1310 | `210552620d47aa41f1b8b3c70f9e3ecacbe54d0e75cfe9062510ff53a67d1ce2` |
| `docs/uxts01/agent-package/BUILD_PACKAGE.sh` | 4875 | `f96084f5a109e02d9e6357adff3c3f0628d1704190da2ca9bf1c2053ed649858` |
| `docs/uxts01/agent-package/IMPLEMENTATION_GUIDE.md` | 4514 | `f21fef356b5160e747fd5fdf216f9e0c30f39f12ad30c17edaf70484521e0d17` |
| `docs/uxts01/agent-package/README.md` | 2502 | `f501d7efe5ffcd9b9af904be662f2a14087c8ea39b0257041f3027f9f7c96ca6` |
| `docs/uxts01/agent-package/RELEASE_NOTES_A002_A005.md` | 1705 | `15247a891bd9a21a79527b951e4cba77c9be584cf63ef4518cb919cec53f4a5f` |
| `docs/uxts01/agent-package/START_HERE.md` | 1866 | `16e9bdf5d997a941b1f5327c57b0965292ea0980086b6c3ed752fb48a7d1c3f7` |
| `docs/uxts01/agent-package/source/live-frameworks/uats/README.md` | 9413 | `34489e659cd80772696f1a3c6b25fe646f6dd77e8999b07fb8c3e6528301cb5d` |
| `docs/uxts01/agent-package/source/live-frameworks/uats/health.uats.json` | 787 | `82e176441d517a766f9e681de22e54b4f79859949fe0e79dd31c12984948f561` |
| `docs/uxts01/agent-package/source/live-frameworks/uats/uats.schema.json` | 18259 | `d015f150f49466e54f4ae64f13457639ec0b50ce445aea3a0fc2578d9ba50a1a` |
| `docs/uxts01/agent-package/source/live-frameworks/uats/uats_runner.py` | 68282 | `8bfe6ff5a53eec74c3fe4938b871a9f6c0b1e3764faa0dfd968f8a17040f032a` |
| `docs/uxts01/agent-package/source/live-frameworks/upts/README.md` | 17648 | `e7a820cf24482a1fecec9271f908259a13d4992f47d6b4619bf64a1feffc0253` |
| `docs/uxts01/agent-package/source/live-frameworks/upts/go.upts.json` | 6037 | `12aead851309645a72f003fcbd087915cbf50e629074e68c87c836f902fce1ac` |
| `docs/uxts01/agent-package/source/live-frameworks/upts/go_test_fixture.go` | 1965 | `a95030aa689f0f6312c781aa2bf73847f2919a85b09dcac794f9a2069d8e1e98` |
| `docs/uxts01/agent-package/source/live-frameworks/upts/upts.schema.json` | 7898 | `f82beb64d4bc67d9e66b4e57434ec6978c3fd6dfed368efaa858cbeba56e1d98` |
| `docs/uxts01/agent-package/source/live-frameworks/upts/upts_runner.py` | 42010 | `75ac80dc58487152b058e15aae30873224ec33709cfab243a99df528e7418ca3` |
| `docs/uxts01/agent-package/source/original/UXTS_PORTABLE_AGENT_SPEC.md` | 56054 | `fb8529704f580ebeb7a473d7f95df4bb2ecf4af120b81ac1e2e62a973cfaae26` |
| `docs/uxts01/agent-package/source/original/UXTS_PORTABLE_AGENT_SPEC02.md` | 83800 | `5a84c200b5e81e17b8e5d4832bb0ed214172ea8e48522ce1eb31cd2c6f669bd6` |
| `docs/uxts01/agent-package/source/uxts01/DECISION_REGISTER.md` | 2123 | `b806ce07a120bcdb08d35ed495ead5a363169618b2256dae755a21a1b332867f` |
| `docs/uxts01/agent-package/source/uxts01/README.md` | 739 | `0c7a1e242dad7a99e1f692136c48b046a346ffdd39135ce63d27a4f97717f5c3` |
| `docs/uxts01/agent-package/source/uxts01/SOURCE_PROVENANCE_MATRIX.md` | 1422 | `c963a1e49e84a9c8b1d08295c67f47b0c4b29c2cd1def6ef44828955f21eca10` |
| `docs/uxts01/agent-package/source/uxts01/SUPPORTING_FILES.md` | 1449 | `7227c65a8f1a087aab46f81f46acbc3b98ee03c2d7ff15fdc6d7191e21008ef8` |
| `docs/uxts01/agent-package/source/uxts01/UxXTS01_ADOPTION_ROADMAP.md` | 2673 | `31bef6e01455845a5fd9605bc718ca8b13f360ed7c1f4f528ad9136757af289f` |
| `docs/uxts01/agent-package/source/uxts01/UxXTS01_ANALYSIS_TRIAGE_2026-02-28.md` | 3499 | `5ba36c73455ed4bffa38686594e1582e3a4b410915903e152b4cb47a1cd81659` |
| `docs/uxts01/agent-package/source/uxts01/UxXTS01_INTENT_TO_ITEM_REMEDIATION_SPEC.md` | 9606 | `ae50cc3c81572f03abbe2e04ea13e0138d73cab393d28d776a03db7034636ce1` |
| `docs/uxts01/agent-package/source/uxts01/UxXTS01_MERGE_DECISIONS.md` | 2735 | `3820dece3dc0701819d5430e4fe8bc47415805ac271c4bc53162215321091272` |
| `docs/uxts01/agent-package/source/uxts01/UxXTS01_PORTABLE_AGENT_SPEC.md` | 12708 | `1015bbc6fb22536383fa2b04cb804c4b7859801d416e6735c6dda7e167c121ac` |
| `docs/uxts01/agent-package/source/uxts01/UxXTS01_RUNNER_CORE_CONTRACT.md` | 1531 | `5893b6c00498bf5988d221649b98fd6f88abd5abc607765051d9653a17c5383b` |
| `docs/uxts01/agent-package/source/uxts01/WORKING_CONTEXT_LOG.md` | 3794 | `4f104f29b64e14f2be340b98baf5827ca61e23586cdf4f2b472549be73faa61a` |
| `docs/uxts01/agent-package/source/uxts01/conformance/conformance-suite.json` | 12809 | `16f0cbed49adb30a6052e64b53eb17b1adc35b002f1e99665c91a209d02784ca` |
| `docs/uxts01/agent-package/source/uxts01/examples/frameworks/uats/_defaults.json` | 355 | `6a772c7b9bc2877106e2ef11092b80391a7b65a1d910d1f2d28edbab2a1c4966` |
| `docs/uxts01/agent-package/source/uxts01/examples/frameworks/uats/fixtures/health/expected.json` | 79 | `e9a0d9643e75dc4e2260ed79a48b774d20d4ff66618b62ac90d9ecaf61f4cd0a` |
| `docs/uxts01/agent-package/source/uxts01/examples/frameworks/uats/schema.json` | 2342 | `e660240372a0a301e6834a0ff11a75d90f95d9238e7e57bd0881f77b63100deb` |
| `docs/uxts01/agent-package/source/uxts01/examples/frameworks/uats/specs/health.uats.json` | 753 | `edd9dfd28c36aa77e6e08ac6cceb7b7c61bdd59a5ef3e722ba56716026f4b30e` |
| `docs/uxts01/agent-package/source/uxts01/examples/sample_uxxts_report.json` | 694 | `300cd664db138c7b0722a17cdc3f34dda3089d331c6426cbac83696801160a98` |
| `docs/uxts01/agent-package/source/uxts01/examples/schemas/uxxts-common.schema.json` | 3689 | `e892646e5b3bce16f996e645b766d17b6f8df7b79eefa16dddb9d7a9c8379d18` |
| `docs/uxts01/agent-package/source/uxts01/schemas/uxxts-common.schema.json` | 3689 | `e892646e5b3bce16f996e645b766d17b6f8df7b79eefa16dddb9d7a9c8379d18` |
| `docs/uxts01/agent-package/source/uxts01/schemas/uxxts-report-aggregate.schema.json` | 5451 | `797dc7fe5ea7a26f17b6882bafdb23494f8991d9f7db3797d9ca7b0a2abc2288` |
| `docs/uxts01/agent-package/source/uxts01/schemas/uxxts-report.schema.json` | 6118 | `ff4420a45156541ab1afe976645a5c74cda1728ea73569b25a0401fa905ae941` |
| `docs/uxts01/agent-package/source/uxts01/tools/uxxts_init.py` | 2623 | `3e33996efc6b01092dae7225d3f5c76013c4de703f3be625b395fa7762c43654` |
| `docs/uxts01/agent-package/source/uxts01/tools/uxxts_lint.py` | 6908 | `531cd919ed1542d9dabda8354c449f14ef9103b77aca3fcc58edb54307daf69e` |
