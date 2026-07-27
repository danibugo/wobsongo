# Project Charter: Wobsongo 

## 1. Vision

To create a standardized, accessible, and transparent infrastructure for automated truth verification, empowering organizations globally to combat misinformation in their local contexts.

## 2. Governance

- **Project Lead:** [ImpactScope team](https://impactscope.com) as KAIROS' technical partner (maintains core roadmap and release cycle).
- **Maintainers:** [ImpactScope team](https://impactscope.com) is responsible for reviewing Pull Requests and patching security vulnerabilities.
- **Community:** Open to contributions from NGOs, researchers, and developers.

## 3. Sustainability Plan

This project is the core engine of the commercial Wobsongo Platform. KAIROS is contractually committed to maintaining this open-source repository as part of its business operations, ensuring long-term updates and bug fixes.

## 4. Roadmap

- **Q1 2026:** Release generic interfaces for Vector DBs (Weaviate, Qdrant).
- **Q2 2026:** Add support for local LLMs (Llama 3, Mistral) for offline usage.
- **Q3 2026:** Release benchmark datasets for low-resource languages (Moore, Fula).
- **Q4 2026:** Release a web-based demo platform for easy testing and deployment, stable v1.0.0 release.

## 5. DPG Standards Compliance

- **Open Source:** Apache 2.0 License.
- **Open Standards:** Uses JSON for data interchange and standard SQL/Vector interfaces.
- **Do No Harm:** The system includes safety guardrails in the default prompts to prevent the generation of harmful content.
- **Licensing Strategy:** **Permissive (Apache 2.0)**.
  _Reasoning:_ We chose a permissive license to maximize adoption by governments, NGOs, and private sector partners. This allows the "Wobsongo Verify" engine to be integrated into diverse software ecosystems (both open and proprietary) without legal friction, ensuring the widest possible impact for SDG 3.
