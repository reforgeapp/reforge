package deployment

import "slices"

func SafeRecovery(current Provenance, restoreArtifact string) bool {
	proof := current.Recovery
	return proof != nil && (proof.State == "stateless" || proof.State == "compatible" && slices.Contains(proof.CompatibleArtifacts, restoreArtifact))
}
