# UCSPEC development

Read README.md, docs/architecture.md, docs/authoring.md and docs/verification.md before modifying semantics.
Keep the finite execution engine independent of layout, file formats and CLI orchestration.
Do not substitute fixture placement for elk-go in production. Report integration/build limits honestly.
Use the shared product state model; use cases refer to transitions and have independently defined scopes.
A passing model check is not an implementation test. An incomplete search is not a successful proof.
Never lower thresholds, remove independent properties, narrow domains or auto-approve generated paths merely to turn a failure green.
Run core tests and full integration tests when dependencies are available. Windows updating requires the Windows-specific test.
Keep embedded guides synchronized with docs and init templates synchronized with sample specifications.
No network, arbitrary code, or implicit latest-version updates in analysis. Updates are explicit commands only.
