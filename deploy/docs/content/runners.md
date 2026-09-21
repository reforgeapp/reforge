# Runners

Runners execute repository and model work outside the control plane.

## Pools

A pool may start empty. Create a pool, then enrol a runner. Pool updates invalidate
existing enrolments when the scope changes.

## Enrolment

Enrolment produces a one-time instruction that expires. There is no permanent token in a
downloadable file. The runner registers outbound, so no inbound port is required.

## Placement

Run placement explains why a private repository or subscription agent requires a specific
pool. A private repository needs a pool bound to its approved private route.

## Drain and rotation

Draining a pool stops new claims while in-flight work finishes. Rotate a runner by
revoking it and enrolling a replacement; revocation fences any late upload or publish.

## Host prerequisites

The runner needs an OCI runtime and, for untrusted repositories, a sandbox with working
resource enforcement. On hosts without cgroup delegation the sandbox reports unavailable
and hostile repositories must not be admitted. See
[Support matrix and limitations](support-matrix.md).
