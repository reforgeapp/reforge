# Reforge

Reforge uses LLMs to fully or partially manage software repositories. The goal:
create an application, deploy it, and let Reforge handle ongoing maintenance.

Choose how much to delegate, from proposed fixes to autonomous maintenance.
For repositories under autopilot, Reforge acts as the delegated owner,
managing existing work and its own changes within configured permissions and
budgets.

Reforge combines repository scanning, dependency and CI repair, validation,
change publication, and merge controls. Deployment and GitOps capabilities
are also present; connecting these into reliable end-to-end delivery and ongoing
production recovery is still in development.

Built with Go, React/TypeScript, PostgreSQL, and execution runners.
