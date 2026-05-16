// SPDX-Licence-Identifier: EUPL-1.2

// Event name constants for the deploys surface.
// v1 emits EventDeployCreated on every successful Create call.
// EventDeployUpdated is reserved for v2 when rollback and status-sync are added.

package deploys

// EventDeployCreated fires when a new deploy record is persisted.
// Payload is the CreateOutput containing the generated deploy ID.
const EventDeployCreated = "deploys.deploy.created"

// EventDeployUpdated fires when an existing deploy record is mutated (v2).
// Payload is the DeployRecord after the update.
const EventDeployUpdated = "deploys.deploy.updated"
