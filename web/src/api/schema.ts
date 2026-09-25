export interface paths {
    "/healthz": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["health"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/readyz": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["readiness"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/meta": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["metadata"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/session": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["session"];
        put?: never;
        post?: never;
        delete: operations["revokeSessions"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["repositories"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/login": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["login"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/callback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["oidcCallback"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/logout": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["logout"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/bootstrap": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["bootstrap"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/memberships": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listMemberships"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/memberships/{userID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["putMembership"];
        post?: never;
        delete: operations["deleteMembership"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/teams": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listTeams"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/teams/{teamID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["putTeam"];
        post?: never;
        delete: operations["deleteTeam"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listConnections"];
        put?: never;
        post: operations["createConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getConnection"];
        put?: never;
        post?: never;
        delete: operations["revokeConnection"];
        options?: never;
        head?: never;
        patch: operations["updateConnection"];
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/delete": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["deleteConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/test": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["testConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/rotate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["rotateCredential"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/private-route": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["setPrivateRoute"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/forges": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listForgesConnections"];
        put?: never;
        post: operations["createForgesConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/models": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listModelsConnections"];
        put?: never;
        post: operations["createModelsConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/model-catalog": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["listModelCatalog"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/agents": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listAgentsConnections"];
        put?: never;
        post: operations["createAgentsConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/delivery": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listDeliveryConnections"];
        put?: never;
        post: operations["createDeliveryConnection"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/rewrap": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["rewrapCredential"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/policies/effective": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getEffectivePolicy"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repoID}/policy": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getRepositoryPolicy"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/policies/versions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listPolicyVersions"];
        put?: never;
        post: operations["createPolicyVersion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/policies/versions/{versionID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getPolicyVersion"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/policies/versions/{versionID}/simulate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["simulatePolicy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/policies/versions/{versionID}/activate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["activatePolicy"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/tasks": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listTasks"];
        put?: never;
        post: operations["enqueueTask"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/tasks/{taskID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getTask"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/tasks/{taskID}/cancel": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["cancelTask"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/tasks/{taskID}/resume": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["resumeTask"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/pauses/{scopeKind}/{scopeID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["setPause"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/events/replay": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["replayEvents"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["streamEvents"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/budgets/{scopeKind}/{scopeID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getBudget"];
        put: operations["putBudget"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/budget-routes/{connectionID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getBudgetRoute"];
        put: operations["putBudgetRoute"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/usage/{reservationID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getReservation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/runner-pools": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listRunnerPools"];
        put?: never;
        post: operations["createRunnerPool"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/runner-pools/{poolID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getRunnerPool"];
        put: operations["updateRunnerPool"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/runner-pools/{poolID}/enrollments": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["createRunnerEnrollment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/runner-pools/{poolID}/runners": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listRunners"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/runners/{runnerID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete: operations["revokeRunner"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/artifacts/{artifactID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["artifactMetadata"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/artifacts/{artifactID}/download": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["downloadArtifact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/enroll": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["enrollRunner"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/rotate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["rotateRunner"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/claim": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["claimRunnerJob"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/heartbeat": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerHeartbeat"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/progress": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerProgress"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/result": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerResult"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/artifacts": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["uploadRunnerArtifact"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/artifacts/{artifactID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["downloadRunnerArtifact"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/operations/{operation}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["invokeRunnerOperation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/private/poll": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;

        post: operations["pollPrivateGrant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/private/results": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["completePrivateGrant"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/inventory-syncs": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listInventoryJobs"];
        put?: never;
        post: operations["startInventorySync"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/inventory-syncs/{syncID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getInventoryJob"];
        put?: never;
        post?: never;
        delete: operations["cancelInventoryJob"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/inventory-syncs/{syncID}/candidates": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listInventoryCandidates"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/inventory-syncs/{syncID}/import": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["importInventoryCandidates"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getInventoryRepository"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/changes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listInventoryChanges"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/webhook": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getInventoryWebhook"];
        put: operations["rotateInventoryWebhook"];
        post?: never;
        delete: operations["revokeInventoryWebhook"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/hooks/v1/{orgID}/{endpointID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;

        post: operations["receiveForgeWebhook"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/findings": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listFindings"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/findings/{findingID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getFinding"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch: operations["updateFinding"];
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/findings/advisories": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["importAdvisory"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/maintenance": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getMaintenanceConfig"];
        put: operations["putMaintenanceConfig"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/discovery": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getDiscoveryScan"];
        put?: never;
        post: operations["startDiscoveryScan"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repair-recipes": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listRepairRecipes"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repair-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewRepair"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repair-runs": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["enqueueRepair"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repair-runs/{taskID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getRepairRun"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repair-runs/{taskID}/reconcile": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["reconcileRepair"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/context": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["runnerRepairContext"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/run": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["runnerRepairRun"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/source/{sha}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["runnerRepairSnapshot"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/report": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerRepairReport"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/stage": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerRepairStage"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/publish": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerRepairPublish"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/model-turns": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerModelTurn"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repoID}/repair-baseline": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getRepairBaseline"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/runner/v1/repair/native-checks": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runnerRecordNativeChecks"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/merge-configuration": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getMergeConfiguration"];
        put: operations["putMergeConfiguration"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/changes/{changeID}/merge-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewProtectedMerge"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/merge-operations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listMergeOperations"];
        put?: never;
        post: operations["requestProtectedMerge"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/merge-operations/{operationID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getMergeOperation"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/merge-operations/{operationID}/cancel": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["cancelMergeOperation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/merge-operations/{operationID}/reconcile": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["reconcileMergeOperation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/changes/{changeID}/revalidations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listBotRevalidations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployment-configurations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listDeploymentConfigurations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployment-configurations/{environment}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["putDeploymentConfiguration"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployment-configurations/{environment}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewDeployment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployments": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listDeployments"];
        put?: never;
        post: operations["requestDeployment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployments/{deploymentID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getDeployment"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployments/{deploymentID}/observe": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["observeDeployment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/deployment-health/{orgID}/{deploymentID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["submitDeploymentHealth"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployments/{deploymentID}/cancel": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["cancelDeployment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repoID}/delivery-workflows": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listDeliveryWorkflows"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/deployment-configurations/{environment}/track": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["trackDeployment"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-configurations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listGitOpsConfigurations"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-configurations/{environment}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put: operations["putGitOpsConfiguration"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-configurations/{environment}/preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewGitOpsPromotion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listGitOpsPromotions"];
        put?: never;
        post: operations["requestGitOpsPromotion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions/{promotionID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getGitOpsPromotion"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions/{promotionID}/cancel": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["cancelGitOpsPromotion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions/{promotionID}/observe": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["observeGitOpsPromotion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions/{promotionID}/continue": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["continueGitOpsPromotion"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions/{promotionID}/merge-preview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewGitOpsMerge"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/gitops-promotions/{promotionID}/merge": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["requestGitOpsMerge"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/gitops-health/{orgID}/{promotionID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["submitGitOpsHealth"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/usage": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listUsage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/overview": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getOverview"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/setup": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getSetup"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/usage/summary": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["summarizeUsage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/usage/series": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["seriesUsage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/audit-events": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listAuditEvents"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/audit-events/export": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };

        get: operations["exportAuditPage"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/connections/{connectionID}/agent-qualification": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getAgentQualification"];
        put: operations["putAgentQualification"];
        post?: never;
        delete: operations["deleteAgentQualification"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/custom-profiles": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listCustomProfiles"];
        put?: never;
        post: operations["createCustomProfile"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/custom-profiles/{profileID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getCustomProfile"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/custom-profiles/{profileID}/approve": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["approveCustomProfile"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/custom-profiles/{profileID}/revoke": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["revokeCustomProfile"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaign-previews": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["previewCampaign"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listCampaigns"];
        put?: never;
        post: operations["createCampaign"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns/{campaignID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getCampaign"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns/{campaignID}/members": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["listCampaignMembers"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns/{campaignID}/start": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["startCampaign"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns/{campaignID}/pause": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["pauseCampaign"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns/{campaignID}/resume": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["resumeCampaign"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/campaigns/{campaignID}/cancel": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["cancelCampaign"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/identity/oidc": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };

        get: operations["getOrgOIDC"];

        put: operations["putOrgOIDC"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/identity/oidc/probe": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;

        post: operations["probeOrgOIDC"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/identity/oidc/activate": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;

        post: operations["activateOrgOIDC"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/identity/oidc/disable": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;

        post: operations["disableOrgOIDC"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/identity/oidc/invitations": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };

        get: operations["listOrgOIDCInvitations"];
        put?: never;

        post: operations["createOrgOIDCInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/identity/oidc/invitations/{invitationID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;

        delete: operations["revokeOrgOIDCInvitation"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/invitations/redeem": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;

        post: operations["redeemOrgOIDCInvitation"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/github-app": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getGitHubAppSetup"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/github-app/setups": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["startGitHubAppSetup"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/github-app/setups/{setupID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post?: never;
        delete: operations["cancelGitHubAppSetup"];
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/github/setup/{setupID}": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["githubAppHandoff"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/github/manifest/callback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["githubManifestCallback"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/github/install/callback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["githubInstallCallback"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/auth/github/oauth/callback": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["githubOAuthCallback"];
        put?: never;
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/hooks/github/app": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["githubAppWebhook"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/autopilot": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get: operations["getAutopilot"];
        put: operations["updateAutopilot"];
        post?: never;
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
    "/api/v1/orgs/{orgID}/repositories/{repositoryID}/run": {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        get?: never;
        put?: never;
        post: operations["runRepository"];
        delete?: never;
        options?: never;
        head?: never;
        patch?: never;
        trace?: never;
    };
}
export type webhooks = Record<string, never>;
export interface components {
    schemas: {
        APIError: {
            code: string;
            message: string;
            request_id: string;
            retryable: boolean;
            details?: {
                [key: string]: unknown;
            };
        };
        CustomProfile: {
            id: string;
            name: string;
            version: number;
            image_digest: string;
            executable: string;
            argv: string[];
            protocol_version: number;
            max_wall_seconds: number;
            max_output_bytes: number;
            max_turns: number;
            concurrency: number;
            approval_evidence: string;
            approved_by?: string;
            approved_at?: string;
            revoked_at?: string;
            created_at: string;
        };
        CustomProfileInput: {
            name: string;
            image_digest: string;
            executable: string;
            argv: string[];
            protocol_version: number;
            max_wall_seconds: number;
            max_output_bytes: number;
            max_turns: number;
            concurrency: number;
        };
        CustomProfilePage: {
            items: components["schemas"]["CustomProfile"][];
            next_cursor?: string;
            complete: boolean;
        };
        CustomProfileApproval: {
            evidence: string;
        };
        AgentQualification: {
            binding: components["schemas"]["AgentBinding"];
            evidence_id: string;
            checked_at: string;
            expires_at: string;
            auth_custody: boolean;
            native_tool_containment: boolean;
            terms: boolean;
            topology: boolean;
            entitlement: boolean;
            quota: boolean;
            no_paid_overage: boolean;
        };
        AgentBinding: {
            org_id: string;
            connection_id: string;
            runner_id?: string;
            connection_version: number;
            credential_version: number;
            account_id: string;
            model: string;
            runtime_digest: string;
            deployment: string;
        };
        AgentQualificationInput: {
            evidence_id: string;
            checked_at: string;
            expires_at: string;
            auth_custody: boolean;
            native_tool_containment: boolean;
            terms: boolean;
            topology: boolean;
            entitlement: boolean;
            quota: boolean;
            no_paid_overage: boolean;
        };
        AgentQualificationResponse: {
            qualification?: components["schemas"]["AgentQualification"];
            capabilities: {
                [key: string]: components["schemas"]["Capability"];
            };
        };
        Overview: {
            counts: components["schemas"]["OverviewCounts"];
            attention: components["schemas"]["OverviewAttention"][];
            portfolio: components["schemas"]["OverviewPortfolioRow"][];
            capacity: components["schemas"]["OverviewCapacity"];
            trend: components["schemas"]["OverviewDay"][];
            severity: components["schemas"]["OverviewSeverity"][];
        };
        OverviewPortfolioRow: {
            repository_id: string;
            repository_name: string;
            provider: string;
            accessible: boolean;
            open_findings: number;
            open_changes: number;
            blocked: number;
            last_synced_at: string;
            blocker?: string;
        };
        OverviewCapacity: {
            queued_jobs: number;
            running_jobs: number;
            active_pools: number;
            active_runners: number;
            reserved_micro_usd: number;
        };
        OverviewCounts: {
            needs_decision: number;
            running: number;
            ready_for_review: number;
            blocked: number;
            verified_deployments: number;
            accessible_repositories: number;
            stale_repositories: number;
            queued_jobs: number;
        };
        OverviewAttention: {
            id: string;
            repository_id: string;
            repository_name: string;
            title: string;
            severity: string;
            state: string;
            age_seconds: number;
            assigned_to?: string;
        };
        Meta: {
            name: string;
            version: string;

            edition: "hosted" | "self-hosted";
            development: boolean;
            fixture_auth: boolean;
            bootstrap_required?: boolean;
            docs_url?: string;
        };
        Health: {
            status: string;
        };

        Role: "owner" | "admin" | "maintainer" | "reviewer" | "viewer";
        Capability: {

            state: "supported" | "unsupported" | "unknown";
            scope: string;
            reason: string;
            source: string;
            version: string;

            last_checked: string;
        };
        Organisation: {
            id: string;
            name: string;

            version: number;
            paused: boolean;
        };
        Membership: {
            org_id: string;
            role: components["schemas"]["Role"];
            team_ids: string[];
            repository_ids: string[];
            all_repositories: boolean;
            user_id?: string;

            version?: number;
        };
        Session: {
            user: {
                id: string;
                name: string;
                email: string;
            };
            organisations: components["schemas"]["Organisation"][];
            memberships: components["schemas"]["Membership"][];
            csrf_token: string;
        };
        Repository: {
            id: string;
            org_id: string;
            connection_id: string;
            native_id: string;
            name: string;
            url: string;

            provider: "github" | "gitlab" | "gitea";
            default_branch: string;
            archived: boolean;
            paused: boolean;
            accessible: boolean;
            team_ids: string[];

            last_synced_at: string | null;

            version: number;
            sync_state?: string;
            sync_reason?: string;

            connection_version?: number;

            changes_observed_at?: string;
        };
        RepositoryPage: {
            items: components["schemas"]["Repository"][];
            next_cursor?: string;
            complete: boolean;
        };

        TaskState: "queued" | "reproducing" | "planning" | "repairing" | "validating" | "publishing" | "completed" | "blocked" | "failed" | "cancelling" | "cancelled" | "reconciling";

        ChangeState: "draft" | "open" | "evaluating" | "eligible" | "merge_requested" | "queued" | "merged" | "blocked" | "reconciling" | "closed";

        DeploymentState: "planned" | "awaiting_gates" | "eligible" | "requested" | "running" | "verifying" | "healthy" | "blocked" | "reconciling" | "failed" | "completed_unverified" | "recovery_requested" | "recovering" | "recovered" | "recovery_failed" | "cancelled";
        Decision: {

            outcome: "allow" | "deny" | "unknown";
            policy_hash: string;
            blockers: string[];
            required_actions: string[];
            rules: string[];
        };
        Event: {

            id: number;
            org_id: string;
            repository_id?: string;
            type: string;
            aggregate_type: string;
            aggregate_id: string;

            aggregate_version: number;

            occurred_at: string;
            request_id: string;
            data_version: number;
            data: {
                [key: string]: unknown;
            };
        };
        Team: {
            id: string;
            name: string;
            repository_ids: string[];

            version: number;
        };
        BootstrapRequest: {
            token: string;
            name: string;
        };
        MembershipPage: {
            items: components["schemas"]["Membership"][];
            next_cursor?: string;
            complete: boolean;
        };
        TeamPage: {
            items: components["schemas"]["Team"][];
            next_cursor?: string;
            complete: boolean;
        };
        MembershipInput: {
            role: components["schemas"]["Role"];
            all_repositories: boolean;
            team_ids: string[];
            repository_ids: string[];
        };
        TeamInput: {
            name: string;
            repository_ids: string[];
        };
        ConnectionSettings: {
            namespace?: string;
            model?: string;
            profile?: string;
            auth_kind: string;
            billing_route: string;
            app_id?: string;
            installation_id?: string;
            runtime_version?: string;
            ca_pem?: string;
            allowed_models?: string[];

            readonly managed?: "github_manifest" | "github_hosted";
            readonly webhook_pending?: boolean;
        };
        PrivateRoute: {

            runner_id: string;
            host: string;
            cidrs: string[];

            readonly approved_at?: string;

            readonly revoked_at?: string | null;
        };
        ConnectionCapability: {
            state: string;
            scope: string;
            reason: string;
            source: string;
            version: string;

            last_checked: string;
        };
        Connection: {
            id: string;
            org_id: string;
            kind: string;
            provider: string;
            name: string;
            endpoint: string;
            state: string;
            reason: string;
            server_version: string;
            settings: components["schemas"]["ConnectionSettings"];
            capabilities: {
                [key: string]: components["schemas"]["ConnectionCapability"];
            };

            verified_at: string | null;

            credential_version: number;

            version: number;
            private_route?: components["schemas"]["PrivateRoute"];

            key_confirmed_at?: string;
        };
        ConnectionCreate: {
            kind: string;
            provider: string;
            name: string;
            endpoint: string;
            settings: components["schemas"]["ConnectionSettings"];
            private_route?: components["schemas"]["PrivateRoute"];
            secret?: string;
        };
        ConnectionPage: {
            items: components["schemas"]["Connection"][];
            next_cursor?: string;
            complete: boolean;
        };
        CredentialRotation: {
            secret: string;
        };
        PrivateRouteChange: {
            route: components["schemas"]["PrivateRoute"];
        };
        PolicyScope: {

            kind: "organisation" | "team" | "repository";
            id: string;
        };
        PolicyLists: {
            recipes?: string[];
            models?: string[];
            routes?: string[];
            merge_methods?: string[];
            environments?: string[];
            workflows?: string[];
        };
        PolicyLimits: {

            budget?: number;

            concurrency?: number;

            attempts?: number;

            changed_files?: number;

            changed_lines?: number;

            open_changes?: number;
        };
        PolicyDefaults: {
            model?: string;
            route?: string;
            branch_prefix?: string;
        };
        PolicyRequirement: {
            id: string;
            identity?: string;
            actions: string[];
            approvals?: number;
        };
        PolicyDocument: {

            schema: "maintenance/v1";
            allow?: components["schemas"]["PolicyLists"];
            deny?: string[];
            forbidden_paths?: string[];
            limits?: components["schemas"]["PolicyLimits"];
            required?: components["schemas"]["PolicyRequirement"][];
            defaults?: components["schemas"]["PolicyDefaults"];

            max_evidence_age_seconds?: number;
            paused?: boolean;
        };
        PolicyLayer: {
            scope: components["schemas"]["PolicyScope"];
            version_id: string;

            binding_version: number;
            policy: components["schemas"]["PolicyDocument"];
        };
        ResolvedPolicy: {
            hash: string;
            layers: components["schemas"]["PolicyLayer"][];
            primary_team_id?: string;
            repository_id: string;
            paused: boolean;
            policy: components["schemas"]["PolicyDocument"];
            scope_paused: boolean;
            missing_defaults: string[];
            problems: string[];
        };
        PolicyEvidenceBinding: {
            head?: string;
            target?: string;
            tested?: string;
            policy_hash?: string;
            provider_rules?: string;
            capability_version?: string;
            source_sha?: string;
            artifact?: string;
        };
        PolicyEvidence: {
            id: string;
            identity?: string;
            state: string;
            approvals?: number;
            binding: components["schemas"]["PolicyEvidenceBinding"];

            observed_at: string;
            reference: string;
        };
        PolicyInput: {
            action: string;
            recipe?: string;
            model?: string;
            route?: string;
            merge_method?: string;
            environment?: string;
            workflow?: string;
            starting_policy_hash?: string;
            paths?: string[];
            usage?: components["schemas"]["PolicyLimits"];
            current?: components["schemas"]["PolicyEvidenceBinding"];
            evidence?: components["schemas"]["PolicyEvidence"][];
            paused_scopes?: string[];

            readonly now?: string;

            stage?: "" | "deployment_admission";
        };
        PolicyResult: {

            outcome: "allow" | "deny" | "unknown";
            policy_hash: string;
            blockers: string[];
            required_actions: string[];
            rules: string[];
            bindings: components["schemas"]["PolicyLayer"][];
            evidence_references: string[];
            starting_policy_hash: string;
        };
        PolicyVersion: {
            id: string;
            scope: components["schemas"]["PolicyScope"];
            policy: components["schemas"]["PolicyDocument"];
            hash: string;
            actor_id: string;
            reason: string;

            created_at: string;
        };
        PolicyVersionPage: {
            items: components["schemas"]["PolicyVersion"][];
            next_cursor?: string;
            complete: boolean;
        };
        PolicySimulation: {
            hash: string;
            resolved: components["schemas"]["ResolvedPolicy"];
            decision: components["schemas"]["PolicyResult"];
        };
        PolicyVersionCreate: {
            scope: components["schemas"]["PolicyScope"];
            policy: components["schemas"]["PolicyDocument"];
            reason: string;
        };
        PolicySimulateRequest: {
            repository_id?: string;
            primary_team_id?: string;
            input: components["schemas"]["PolicyInput"];
        };
        PolicyActivateRequest: {
            repository_id?: string;
            primary_team_id?: string;
            simulation_hash: string;
            reason: string;
        };
        PolicyBindingVersion: {

            version: number;
        };
        Task: {
            id: string;
            org_id: string;
            repository_id: string;
            operation_id: string;
            recipe: string;
            recipe_version: string;
            target_branch: string;
            model_connection_id?: string;
            model_route: string;
            campaign_id?: string;
            runner_pool_id?: string;
            policy_hash: string;
            starting_policy_hash: string;
            reason: string;
            state: components["schemas"]["TaskState"];

            version: number;

            cancel_version: number;

            max_attempts: number;

            created_at: string;
            cancellation_requested: boolean;
        };
        TaskPage: {
            items: components["schemas"]["Task"][];
            next_cursor?: string;
            complete: boolean;
        };
        TaskCreate: {
            repository_id: string;
            recipe: string;
            recipe_version: string;
            target_branch: string;
            model_connection_id?: string;
            model_route?: string;
            campaign_id?: string;
            runner_pool_id?: string;
            policy_hash?: string;
            idempotency_key: string;

            max_attempts?: number;

            priority?: number;
        };
        Pause: {
            kind: string;
            id: string;
            paused: boolean;

            version: number;
        };
        PauseInput: {
            paused: boolean;
        };
        EventPage: {
            items: components["schemas"]["Event"][];

            cursor: number;
            complete: boolean;
        };
        BudgetScope: {
            kind: string;
            id: string;
        };
        BudgetAmount: {

            micro_usd: number;

            tokens: number;

            milliseconds: number;

            requests: number;

            concurrency: number;
        };
        BudgetCaps: {

            micro_usd?: number | null;

            tokens?: number | null;

            milliseconds?: number | null;

            requests?: number | null;

            concurrency?: number | null;
        };
        BudgetLimit: {
            scope: components["schemas"]["BudgetScope"];

            period: "daily" | "monthly" | "custom";

            start?: string;

            end?: string;
            caps: components["schemas"]["BudgetCaps"];
            paused: boolean;

            version: number;
            held: components["schemas"]["BudgetAmount"];
            spent: components["schemas"]["BudgetAmount"];
        };
        BudgetLimitInput: {

            period: "daily" | "monthly" | "custom";

            start?: string;

            end?: string;
            caps: components["schemas"]["BudgetCaps"];
            paused: boolean;
        };
        BudgetRoute: {
            connection_id: string;
            model: string;
            name: string;
            mode: string;
            pricing_version: string;
            qualification_ref: string;

            input_micro_usd_per_million: number;

            output_micro_usd_per_million: number;

            request_micro_usd: number;

            max_input_tokens: number;

            max_output_tokens: number;

            max_milliseconds: number;

            max_requests: number;

            version: number;
            qualified: boolean;
            paused: boolean;
        };
        BudgetRouteInput: {
            model: string;
            name: string;
            mode: string;
            pricing_version: string;

            input_micro_usd_per_million: number;

            output_micro_usd_per_million: number;

            request_micro_usd: number;

            max_input_tokens: number;

            max_output_tokens: number;

            max_milliseconds: number;

            max_requests: number;
            paused: boolean;
        };
        WorkflowLease: {
            org_id: string;
            repository_id: string;
            task_id: string;
            job_id: string;
            attempt_id: string;
            operation_id: string;
            worker_id: string;
            policy_hash: string;

            fence: number;

            expires_at: string;
        };
        BudgetQuote: {
            operation_id: string;
            model: string;
            route: string;

            route_version: number;

            input_tokens: number;

            max_output_tokens: number;

            max_milliseconds: number;

            max_requests: number;
        };
        BudgetScopeSnapshot: {
            scope: components["schemas"]["BudgetScope"];

            version: number;

            period_start: string;
        };
        BudgetReservation: {
            id: string;
            connection_id: string;
            campaign_id?: string;
            state: string;
            reference?: string;
            lease: components["schemas"]["WorkflowLease"];
            quote: components["schemas"]["BudgetQuote"];

            connection_version: number;
            route: components["schemas"]["BudgetRoute"];
            scopes: components["schemas"]["BudgetScopeSnapshot"][];
            maximum: components["schemas"]["BudgetAmount"];
            actual?: components["schemas"]["BudgetAmount"];
            debt: components["schemas"]["BudgetAmount"];

            created_at: string;

            dispatched_at?: string;
        };
        RunnerPoolInput: {
            name: string;

            state?: "active" | "draining" | "revoked";
            repository_ids: string[];
        };
        RunnerPool: {
            name: string;

            state: "active" | "draining" | "revoked";
            repository_ids: string[];

            id: string;

            org_id: string;

            version: number;
            runner_count?: number;
            busy_slots?: number;
            builtin?: boolean;
        };
        Runner: {

            id: string;

            org_id: string;

            pool_id: string;
            name: string;
            state: string;

            version: number;

            credential_expires_at: string;
            pool_name: string;
            pool_state: string;

            last_seen_at: string;

            enrolled_at: string;
            busy_slots: number;
            route_count: number;
        };
        EnrollmentToken: {
            token: string;

            expires_at: string;
        };
        RunnerCredential: {
            token: string;

            expires_at: string;
            runner: components["schemas"]["Runner"];
        };
        RunnerAssignment: {
            token: string;

            expires_at: string;
            lease: components["schemas"]["WorkflowLease"];
            task: components["schemas"]["Task"];
        };
        RunnerHeartbeat: {
            lease: components["schemas"]["WorkflowLease"];
            stop: boolean;
            reason?: string;
        };
        ArtifactMetadata: {

            id: string;

            org_id: string;

            repository_id: string;

            task_id: string;
            name: string;
            media_type: string;

            size: number;
            sha256: string;

            created_at: string;

            expires_at: string;
        };
        RunnerPoolPage: {
            items: components["schemas"]["RunnerPool"][];
            next_cursor?: string;
            complete: boolean;
        };
        RunnerPage: {
            items: components["schemas"]["Runner"][];
            next_cursor?: string;
            complete: boolean;
        };
        RunnerCompletion: {

            outcome: "completed" | "failed" | "cancelled" | "uncertain";
            retryable?: boolean;
        };
        PrivateOperation: {

            id: string;

            kind: "forge.probe" | "forge.inventory" | "forge.repository" | "forge.resolve_ref" | "forge.read_file" | "forge.read_change" | "forge.checks" | "forge.approvals" | "forge.reconcile_changes" | "model.probe" | "model.list" | "forge.source_manifest";
            inventory?: {
                [key: string]: unknown;
            };
            repository?: {
                [key: string]: unknown;
            };
            ref?: {
                [key: string]: unknown;
            };
            file?: {
                [key: string]: unknown;
            };
            change?: {
                [key: string]: unknown;
            };
            checks?: {
                [key: string]: unknown;
            };
            changes?: {
                [key: string]: unknown;
            };
            source?: {
                [key: string]: unknown;
            };
        };
        PrivateGrant: {

            id: string;

            runner_version?: number;
            target: {

                org_id: string;

                runner_id: string;
            };
            operation: components["schemas"]["PrivateOperation"];

            authority_id: string;
            connection: {
                [key: string]: unknown;
            };
            timeout_ms: number;

            expires_at: string;

            secret: string;

            result_capability: string;
        };
        PrivateCompletion: {

            grant_id: string;
            result: {
                [key: string]: unknown;
            };
        };
        ForgeRepoRef: {
            native_id: string;
            full_name: string;
        };
        ForgeRepository: {
            native_id: string;
            full_name: string;
            url: string;
            clone_url: string;
            default_branch: string;
            archived: boolean;
            private: boolean;
            permissions: string[] | null;
        };
        ForgeChange: {
            id: string;
            title: string;
            body: string;
            url: string;
            head_sha: string;
            target_sha: string;
            head_branch: string;
            target_branch: string;
            author_id: string;
            author_login: string;
            author_type: string;
            state: string;
            merge_sha: string;
            merge_status: string;
            operation_id: string;
            repository: components["schemas"]["ForgeRepoRef"];
            head_repository: components["schemas"]["ForgeRepoRef"];
            target_repository: components["schemas"]["ForgeRepoRef"];
            draft: boolean;
        };
        InventoryJob: {
            id: string;
            org_id: string;
            connection_id: string;
            namespace: string;

            kind: "scan" | "import" | "refresh";
            repository_id?: string;
            parent_id?: string;

            state: "queued" | "running" | "complete" | "stale" | "failed" | "cancelled";
            reason?: string;

            connection_version: number;

            pages: number;

            processed: number;

            failures: number;

            version: number;

            available_at: string;

            created_at: string;

            updated_at: string;
        };
        InventorySyncInput: {
            connection_id: string;
            namespace?: string;
        };
        InventoryImportInput: {
            all?: boolean;
            native_ids?: string[];
            team_ids?: string[];
        };
        InventoryWebhook: {
            id: string;
            connection_id: string;

            version: number;
            path: string;
            revoked: boolean;
        };
        IssuedInventoryWebhook: {
            id: string;
            connection_id: string;

            version: number;
            path: string;
            revoked: boolean;

            secret: string;
        };
        InventoryJobPage: {
            items: components["schemas"]["InventoryJob"][];
            next_cursor?: string;
            complete: boolean;
        };
        InventoryCandidatePage: {
            items: components["schemas"]["ForgeRepository"][];
            next_cursor?: string;
            complete: boolean;
        };
        InventoryChangePage: {
            items: components["schemas"]["ForgeChange"][];
            next_cursor?: string;
            complete: boolean;
            snapshot_state: string;

            connection_version: number;

            observed_at?: string;
        };
        MaintenanceDependency: {
            ecosystem: string;
            manifest: string;
            name: string;
            from: string;
            to: string;
        };
        MaintenanceBotIdentity: {

            kind: "renovate" | "dependabot";
            actor_id: string;
        };
        MaintenanceBotStatus: {
            present: boolean;

            automerge: "enabled" | "disabled" | "unknown";
        };
        MaintenanceBotConfig: {
            renovate: components["schemas"]["MaintenanceBotStatus"];
            dependabot: components["schemas"]["MaintenanceBotStatus"];
        };
        MaintenanceConfig: {

            repository_id: string;
            trusted_bots: components["schemas"]["MaintenanceBotIdentity"][];

            merge_authority: "observe" | "reforge" | "bot";

            version: number;
        };
        MaintenanceEvidence: {
            provenance: string;

            connection_id: string;

            connection_version: number;

            config_version: number;
            head_sha: string;
            target_sha: string;
            target_branch: string;
            change?: components["schemas"]["ForgeChange"];
            checks: {
                id: string;
                name: string;
                publisher_id: string;
                head_sha: string;
                status: string;
                conclusion: string;
                url: string;
            }[];
            dependencies: components["schemas"]["MaintenanceDependency"][];
            bot?: string;
            ownership: string;
            head_ownership?: string;
            merge_blockers?: string[];
            bot_config?: components["schemas"]["MaintenanceBotConfig"];
            complete: boolean;
            blockers: string[];
            advisory_id?: string;
            reference_url?: string;
        };
        Finding: {

            id: string;

            org_id: string;

            repository_id: string;
            source: string;
            source_id: string;
            category: string;

            severity: "info" | "low" | "medium" | "high" | "critical";
            title: string;
            evidence: components["schemas"]["MaintenanceEvidence"];
            fingerprint: string;
            evidence_digest: string;

            state: "open" | "dismissed" | "snoozed" | "resolved" | "superseded";
            reason: string;

            assigned_to?: string;

            snooze_until?: string;

            superseded_by?: string;

            version: number;

            first_seen: string;

            last_seen: string;
        };
        FindingPage: {
            items: components["schemas"]["Finding"][];
            complete: boolean;
            next_cursor?: string;
        };
        FindingUpdate: {

            action: "assign" | "dismiss" | "snooze" | "reopen";
            reason?: string;
            assigned_to?: string;

            snooze_until?: string;
        };
        AdvisoryInput: {
            advisory_id: string;
            title: string;
            severity: string;
            reference_url: string;
            path: string;
            package: string;
            ecosystem: string;
            affected_range: string;
            commit_sha: string;

            repository_id: string;
        };
        DiscoveryScan: {

            repository_id: string;

            state: "not_started" | "queued" | "running" | "complete" | "failed" | "stale";
            reason: string;

            version: number;

            observed_at?: string;
        };
        RepairInput: {

            finding_id: string;

            finding_version: number;
            recipe: string;

            model_connection_id: string;
            model_route: string;

            runner_pool_id: string;

            custom_profile_id?: string;

            custom_profile_version?: number;
            plan_digest?: string;
            idempotency_key?: string;
        };
        RepairCommand: {
            id: string;
            args: string[] | null;
            directory: string;

            timeout_seconds: number;
            report_format: string;
        };
        RepairRecipe: {
            name: string;
            version: string;
            commands: components["schemas"]["RepairCommand"][] | null;
            protected_paths: string[] | null;
            manifest_paths: string[] | null;

            minimum_tests: number;

            max_files: number;

            max_patch_bytes: number;

            max_turns: number;

            timeout_seconds: number;
        };
        RepairPlan: {
            authority_hash?: string;

            max_changed_lines: number;

            version: number;
            baseline_sha: string;
            target_sha: string;
            image: string;
            recipe: components["schemas"]["RepairRecipe"];
            protected_hashes: {
                [key: string]: string;
            };
            forbidden_paths: string[] | null;
            digest: string;
        };
        RepairCheck: {
            command_id: string;

            exit_code: number;
            output_sha256: string;
            complete: boolean;
            cases: {
                [key: string]: string;
            };
            reason: string;
            excerpt?: string;
        };
        RepairPatch: {
            path: string;

            content: string;
            delete?: boolean;
        };
        RepairReport: {
            diff?: string;
            plan_digest: string;
            state: string;
            reason: string;
            baseline: components["schemas"]["RepairCheck"][] | null;
            candidate: components["schemas"]["RepairCheck"][] | null;
            target: components["schemas"]["RepairCheck"][] | null;
            patches: components["schemas"]["RepairPatch"][] | null;
            artifacts: string[] | null;

            turns: number;
        };
        RepairExecution: {

            max_attempts: number;
            native_head_sha?: string;
            request: components["schemas"]["RepairInput"];
            plan: components["schemas"]["RepairPlan"];
            baseline_repository: components["schemas"]["ForgeRepoRef"];
            repository: components["schemas"]["ForgeRepoRef"];

            connection_id: string;

            connection_version: number;
            model: string;

            max_output_tokens: number;

            turn_timeout_ms: number;
            finding: components["schemas"]["Finding"];
            policy_hash: string;
        };
        RepairPreview: {
            context: components["schemas"]["RepairExecution"];
            blockers: string[] | null;

            expires_at: string;
        };
        RepairRun: {
            candidate_artifacts: string[] | null;
            branch: string;
            candidate_sha: string;
            candidate_checks: components["schemas"]["RepairCheck"][] | null;
            change?: components["schemas"]["ForgeChange"];
            task: components["schemas"]["Task"];
            context: components["schemas"]["RepairExecution"];
            report?: components["schemas"]["RepairReport"];
            state: string;

            version: number;

            updated_at: string;
        };
        RepairPublication: {
            head_sha: string;
            plan_digest: string;
            checks: components["schemas"]["RepairCheck"][] | null;
            artifact_ids: string[] | null;
        };
        SourceFile: {
            path: string;

            content: string;
            executable?: boolean;
            delete?: boolean;
        };
        PinnedSnapshot: {
            commit_sha: string;
            complete: boolean;
            manifest_sha256: string;
            files: components["schemas"]["SourceFile"][] | null;
        };
        ModelTool: {
            name: string;
            description: string;
            schema: unknown;
        };
        ModelToolCall: {
            id: string;
            name: string;
            arguments: unknown;
        };
        ModelMessage: {
            role: string;
            text: string;
            tool_calls?: components["schemas"]["ModelToolCall"][] | null;
            tool_call_id?: string;
        };
        ModelUsage: {

            input_tokens: number;

            output_tokens: number;

            cache_tokens: number;

            cache_creation_tokens: number;
            known: boolean;
            source: string;
        };
        ModelTurn: {

            operation_id: string;
            model: string;
            system: string;
            messages: components["schemas"]["ModelMessage"][] | null;
            tools: components["schemas"]["ModelTool"][] | null;

            max_output_tokens: number;
            continuation?: unknown;

            timeout_ms: number;
        };
        ModelTurnResult: {
            provider_id: string;
            text: string;
            tool_calls: components["schemas"]["ModelToolCall"][] | null;
            usage: components["schemas"]["ModelUsage"];
            continuation?: unknown;
            finish_reason: string;
        };
        MergeQualification: {
            provider: string;
            server_version: string;
            evidence_reference: string;
            evidence_sha256: string;

            connection_version: number;

            inspector_version: number;

            verified_at: string;

            expires_at: string;
            exact_head: boolean;
            strict_target: boolean;
            queue_execution_gate: boolean;
            ci_config_sha256?: string;
        };
        MergeConfiguration: {

            repository_id: string;

            version: number;
            enabled: boolean;

            inspector_connection_id?: string;
            check_publishers: {
                [key: string]: string;
            };
            cooperation_reference: string;
            qualification: components["schemas"]["MergeQualification"];
        };
        ForgeCheckRule: {
            name: string;
            publisher_id: string;
        };
        ForgeCheck: {
            id: string;
            name: string;
            publisher_id: string;
            head_sha: string;
            status: string;
            conclusion: string;
            url: string;
        };
        ForgeApproval: {
            id: string;
            actor_id: string;
            state: string;
            head_sha: string;
            dismissed: boolean;
        };
        ForgeRules: {
            state: string;
            reason: string;
            hash: string;
            code_owners_enforced: string;
            strict_target_enforced: string;

            observed_at: string;
            required_checks: components["schemas"]["ForgeCheckRule"][];

            required_approvals: number;
            dismiss_stale_reviews: boolean;
            require_code_owners: boolean;
            require_strict_target: boolean;
            require_queue: boolean;
            actor_can_bypass: boolean;
            allowed_merge_methods: string[];
        };
        ForgeQueueState: {
            id: string;
            state: string;
            head_sha: string;
            tested_sha: string;
            target_sha: string;
        };
        ForgeNativeEligibility: {
            state: string;
            head_sha: string;
            target_sha: string;
            blockers: string[];
        };
        ForgeMergeCapabilities: {
            provider: string;
            server_version: string;
            features: {
                [key: string]: components["schemas"]["Capability"];
            };
        };
        MergeSnapshot: {
            change: components["schemas"]["ForgeChange"];
            rules: components["schemas"]["ForgeRules"];
            checks: components["schemas"]["ForgeCheck"][];
            approvals: components["schemas"]["ForgeApproval"][];
            native: components["schemas"]["ForgeNativeEligibility"];
            queue: components["schemas"]["ForgeQueueState"];
            capabilities: components["schemas"]["ForgeMergeCapabilities"];

            observed_at: string;
            execution_check?: {
                name: string;
                publisher_id: string;
            };
            train_gate?: {
                [key: string]: unknown;
            };
        };
        MergeGate: {

            id: string;

            repository_id: string;

            connection_id: string;

            connection_version: number;

            configuration_version: number;

            changed_lines: number;
            paths: string[];
            method: string;

            expires_at: string;
            snapshot: components["schemas"]["MergeSnapshot"];
            decision: components["schemas"]["PolicyResult"];
            binding: components["schemas"]["PolicyEvidenceBinding"];
            companions: components["schemas"]["MergeCompanion"][];
            phase?: string;
        };
        ForgeMergeResult: {
            state: string;
            native_id: string;
            merge_sha: string;
            head_sha: string;
            url: string;
        };
        MergeOperation: {

            id: string;

            repository_id: string;

            gate_id: string;

            requested_gate_id: string;
            change_id: string;
            native_queue_id?: string;
            state: string;
            reason: string;
            cancel_requested: boolean;

            version: number;

            created_at: string;

            updated_at: string;
            native_result?: components["schemas"]["ForgeMergeResult"];
        };
        MergeOperationPage: {
            items: components["schemas"]["MergeOperation"][];
            complete: boolean;
            next_cursor?: string;
        };
        MergePreviewRequest: {
            method: string;
        };
        MergeRequestInput: {

            gate_id: string;

            idempotency_key: string;
        };
        MergeCompanion: {
            task_id: string;
            change_id: string;
            head_sha: string;
            merge_sha: string;
            state: string;
        };
        BotRevalidation: {

            task_id: string;
            change_id: string;
            companion_id: string;

            state: "pending" | "waiting_companion" | "blocked" | "ready" | "merged" | "closed";
            reason: string;

            observed_at: string | null;
            gate?: components["schemas"]["MergeGate"];
        };
        DeliveryWorkflow: {
            id: string;
            path: string;
            ref: string;
            sha: string;
            config_sha256: string;
            inputs: {
                [key: string]: string;
            };
        };
        DeliveryQualification: {
            provider: string;
            server_version: string;

            connection_version: number;
            evidence_reference: string;
            evidence_sha256: string;

            verified_at: string;

            expires_at: string;
            pinned_inputs: boolean;
            native_enforcement: boolean;
            environment_serialization: boolean;
            no_bypass: boolean;
        };
        DeploymentConfiguration: {
            environment: string;

            repository_id: string;

            version: number;
            enabled: boolean;

            mode: "pipeline" | "observe";
            workflow: components["schemas"]["DeliveryWorkflow"];
            recovery_workflow?: components["schemas"]["DeliveryWorkflow"];
            provenance_public_key: string;
            health_public_key: string;
            health_checks: string[];

            observation_seconds: number;

            max_evidence_age_seconds: number;

            deadline_seconds: number;
            qualification: components["schemas"]["DeliveryQualification"];
            native_environment: string;
        };
        ArtifactProvenance: {

            org_id: string;

            repository_id: string;
            source_sha: string;
            artifact_digest: string;
            build_id: string;

            issued_at: string;

            expires_at: string;
        };
        SignedArtifactProvenance: {
            document: components["schemas"]["ArtifactProvenance"];
            signature: string;
        };
        DeploymentPreviewInput: {
            change_id: string;
            source_sha: string;
            artifact_digest: string;
            provenance: components["schemas"]["SignedArtifactProvenance"];

            recovery_of?: string;

            restore_deployment_id?: string;
        };
        NativeDeploymentGates: {
            state: string;
            native_enforced: string;
            blockers: string[];
            approval_url: string;
            environment: string;
            rules_hash: string;
        };
        NativeDeploymentStatus: {
            id: string;
            state: string;
            source_sha: string;
            workflow_sha: string;
            artifact_digest: string;
            environment: string;
            correlation_id: string;
            workflow_id: string;
            workflow_path: string;
            ref: string;
            event: string;

            run_attempt: number;
            url: string;
            health: string;

            observed_at: string;

            updated_at: string;

            created_at: string;
        };
        DeliveryPipelineRequest: {
            repository: components["schemas"]["ForgeRepoRef"];
            rules_hash: string;
            workflow_id: string;
            workflow_path: string;
            workflow_sha: string;
            config_sha256: string;
            ref: string;
            source_sha: string;
            artifact_digest: string;
            environment: string;

            correlation_id: string;
            inputs: {
                [key: string]: string;
            };
            observe_only: boolean;
            run_id: string;

            requested_at: string;
        };
        DeploymentGate: {

            id: string;
            environment: string;

            repository_id: string;

            connection_id: string;

            connection_version: number;

            configuration_version: number;
            request: components["schemas"]["DeploymentPreviewInput"];
            pipeline: components["schemas"]["DeliveryPipelineRequest"];
            native: components["schemas"]["NativeDeploymentGates"];
            binding: components["schemas"]["PolicyEvidenceBinding"];
            decision: components["schemas"]["PolicyResult"];
            blockers: string[];

            expires_at: string;
        };
        DeploymentOperation: {

            id: string;
            environment: string;

            repository_id: string;

            gate_id: string;
            state: string;
            reason: string;

            version: number;

            requested_by: string;
            native?: components["schemas"]["NativeDeploymentStatus"];

            recovery_of?: string;

            created_at: string;

            updated_at: string;

            finished_at?: string;
            cancel_requested: boolean;

            cancel_state: "" | "pending" | "dispatching" | "uncertain" | "confirmed";
        };
        DeploymentHealth: {

            org_id: string;

            deployment_id: string;
            environment: string;

            configuration_version: number;
            source_sha: string;
            artifact_digest: string;
            run_id: string;

            run_attempt: number;
            revision: string;
            healthy: boolean;
            checks: {
                [key: string]: boolean;
            };

            observed_at: string;

            nonce: string;
        };
        DeploymentDetail: {
            operation: components["schemas"]["DeploymentOperation"];
            gate: components["schemas"]["DeploymentGate"];
            health?: components["schemas"]["DeploymentHealth"];
        };
        DeploymentOperationPage: {
            items: components["schemas"]["DeploymentOperation"][];
            next_cursor?: string;
            complete: boolean;
        };
        DeploymentConfigurationPage: {
            items: components["schemas"]["DeploymentConfiguration"][];
        };
        DeploymentRequestInput: {

            gate_id: string;
            idempotency_key: string;
        };
        NativeDeliveryWorkflow: {
            id: string;
            name: string;
            ref: string;
            path: string;
            url: string;
        };
        DeploymentTrackInput: {
            change_id: string;
            source_sha: string;
            artifact_digest: string;
            provenance: components["schemas"]["SignedArtifactProvenance"];

            recovery_of?: string;

            restore_deployment_id?: string;
            run_id: string;
            idempotency_key: string;
        };
        GitOpsConfiguration: {
            environment: string;
            target_branch: string;
            manifest_path: string;
            pointer: string;
            image_repository: string;
            provenance_public_key: string;
            health_public_key: string;

            source_repository_id: string;

            delivery_repository_id: string;

            version: number;

            observation_seconds: number;

            max_evidence_age_seconds: number;

            deadline_seconds: number;
            enabled: boolean;
            recovery_allowed: boolean;
            health_checks: string[];
        };
        GitOpsPreviewInput: {
            change_id: string;
            source_sha: string;
            artifact_digest: string;
            provenance: components["schemas"]["SignedArtifactProvenance"];

            recovery_of?: string;

            restore_promotion_id?: string;
        };
        GitOpsGate: {

            id: string;

            operation_id: string;

            source_connection_id: string;

            delivery_connection_id: string;

            source_connection_version: number;

            delivery_connection_version: number;
            source_policy_hash: string;
            delivery_policy_hash: string;
            target_sha: string;
            before: string;
            after: string;
            manifest_sha256: string;
            configuration: components["schemas"]["GitOpsConfiguration"];
            request: components["schemas"]["GitOpsPreviewInput"];
            source: {
                native_id: string;
                full_name: string;
            };
            delivery: {
                native_id: string;
                full_name: string;
            };

            patched_manifest: string;
            decision: components["schemas"]["PolicyResult"];
            blockers: string[];

            expires_at: string;
        };
        GitOpsPromotion: {

            id: string;

            source_repository_id: string;

            delivery_repository_id: string;

            gate_id: string;

            requested_by: string;

            recovery_of?: string;
            environment: string;
            state: string;
            reason: string;
            branch: string;
            candidate_sha: string;
            merge_sha: string;

            version: number;
            cancel_requested: boolean;

            created_at: string;

            updated_at: string;

            finished_at?: string;
            change?: components["schemas"]["ForgeChange"];
        };
        GitOpsHealth: {

            org_id: string;

            promotion_id: string;

            nonce: string;
            environment: string;
            source_sha: string;
            artifact_digest: string;
            delivery_revision: string;

            configuration_version: number;
            healthy: boolean;
            checks: {
                [key: string]: boolean;
            };

            observed_at: string;
        };
        GitOpsDetail: {
            promotion: components["schemas"]["GitOpsPromotion"];
            gate: components["schemas"]["GitOpsGate"];
            health?: components["schemas"]["GitOpsHealth"];
        };
        GitOpsConfigurationPage: {
            items: components["schemas"]["GitOpsConfiguration"][];
        };
        GitOpsPromotionPage: {
            items: components["schemas"]["GitOpsPromotion"][];
            complete: boolean;

            next_cursor?: string;
        };
        UsageEntry: {
            reservation: components["schemas"]["BudgetReservation"];
            provider: string;
            recipe: string;
            repository_name: string;
        };
        UsagePage: {
            items: components["schemas"]["UsageEntry"][];
            complete: boolean;
            next_cursor?: string;
        };

        UsageSummary: {

            records: number;

            settled: number;

            unknown: number;

            reserved: number;

            dispatched: number;

            cancelled: number;

            estimated_cost_micro_usd: number;

            known_tokens: number;
            unknown_maximum: components["schemas"]["BudgetAmount"];
            held: components["schemas"]["BudgetAmount"];
        };
        AuditEvent: {
            id: string;
            actor_id: string;
            action: string;
            object_id: string;
            request_id: string;
            repository_id?: string;

            created_at: string;
            data: {
                [key: string]: unknown;
            };
        };
        AuditEventPage: {
            items: components["schemas"]["AuditEvent"][];
            complete: boolean;
            next_cursor?: string;
        };
        CampaignWindow: {
            weekdays: number[];
            start_minute: number;
            end_minute: number;
        };
        CampaignMemberInput: {

            repository_id: string;
            environment?: string;
            repair?: components["schemas"]["RepairInput"];
            pipeline?: components["schemas"]["DeploymentPreviewInput"];
            gitops?: components["schemas"]["GitOpsPreviewInput"];
        };
        CampaignInput: {
            name: string;

            kind: "repair" | "pipeline" | "gitops";
            selection: string;
            members: components["schemas"]["CampaignMemberInput"][];
            canary_ids: string[];
            canary_size: number;
            batch_size: number;
            concurrency: number;

            success: "published" | "merged" | "healthy";

            observation_seconds: number;
            failure_limit: number;
            failure_percent: number;
            windows: components["schemas"]["CampaignWindow"][];

            not_before?: string;
        };
        CampaignMember: {

            id: string;

            repository_id: string;
            repository_name: string;
            repositories: string[];
            group: string;
            canary: boolean;
            input: components["schemas"]["CampaignMemberInput"];
            pins: {
                [key: string]: string;
            };
            state: string;
            reason: string;
            stage: number;
            halt: boolean;

            action_id?: string;

            succeeded_at?: string;
        };
        CampaignCounts: {
            total: number;
            excluded: number;
            pending: number;
            running: number;
            succeeded: number;
            failed: number;
            unknown: number;
        };
        Campaign: {

            id: string;
            name: string;
            kind: string;
            state: string;
            reason: string;

            version: number;
            stage: number;

            requested_by: string;

            grant_expires_at: string;
            spec: components["schemas"]["CampaignInput"];
            counts: components["schemas"]["CampaignCounts"];

            created_at: string;

            updated_at: string;

            observing_since?: string;
        };
        CampaignPreview: {

            id: string;
            hash: string;
            input: components["schemas"]["CampaignInput"];
            members: components["schemas"]["CampaignMember"][];
            blockers: string[];

            expires_at: string;
        };
        CampaignCreateInput: {

            preview_id: string;
            idempotency_key: string;
        };
        CampaignControlInput: {
            reason: string;

            stage_decision?: "continue_current_stage";
        };
        CampaignPage: {
            items: components["schemas"]["Campaign"][];
            complete: boolean;

            next_cursor?: string;
        };
        CampaignMemberPage: {
            items: components["schemas"]["CampaignMember"][];
            complete: boolean;

            next_cursor?: string;
        };
        OrgOIDCSettings: {
            configured: boolean;
            secret_present: boolean;
            issuer?: string;
            client_id?: string;

            status: "unconfigured" | "draft" | "probe_verified" | "active" | "disabled";

            version: number;

            verified_at?: string;
            verified: boolean;
            activation_available: boolean;
            activation_blocked?: string;
        };
        OrgOIDCInput: {
            issuer: string;
            client_id: string;
            client_secret?: string;
        };
        OrgOIDCInvitationInput: {
            email: string;

            role: "owner" | "admin" | "maintainer" | "reviewer" | "viewer";

            expires_at: string;
        };
        OrgOIDCInvitation: {

            id: string;
            email: string;

            role: "owner" | "admin" | "maintainer" | "reviewer" | "viewer";

            expires_at: string;

            created_at: string;
            redeemed: boolean;
        };
        CreatedOrgOIDCInvitation: {

            id: string;
            email: string;

            role: "owner" | "admin" | "maintainer" | "reviewer" | "viewer";

            expires_at: string;

            created_at: string;
            redeemed: boolean;
            redemption_url: string;
        };
        OrgOIDCInvitationPage: {
            items: components["schemas"]["OrgOIDCInvitation"][];

            next_cursor?: string;
            complete: boolean;
        };
        InvitationAuthorization: {
            authorization_url: string;
        };
        ModelCatalogSettings: {
            model?: string;
            profile?: string;

            auth_kind: "api_key";

            billing_route: "direct_api";
            ca_pem?: string;
        };
        ModelCatalogRequest: {

            provider: "openai" | "anthropic" | "google" | "compatible";
            endpoint: string;
            secret?: string;
            settings: components["schemas"]["ModelCatalogSettings"];
        };
        ModelCatalogItem: {
            id: string;
            name: string;
            disabled?: boolean;
            reason?: string;
        };
        ModelCatalog: {
            items: components["schemas"]["ModelCatalogItem"][];
        };
        GitHubAppPending: {
            id: string;

            phase: "created" | "handed_off" | "converting" | "awaiting_install" | "authorizing" | "completing";
            app_slug?: string;

            expires_at: string;
            resume_url?: string;
        };
        GitHubAppStatus: {

            mode: "manifest" | "hosted" | "unavailable";
            reason?: string;
            pending?: components["schemas"]["GitHubAppPending"];
        };
        GitHubAppSetupCreate: {
            name: string;
            github_org?: string;
        };
        GitHubAppSetup: {
            id: string;
            handoff_url: string;
        };
        UsageDay: {
            day: string;

            micro_usd: number;

            tokens: number;

            records: number;
        };
        UsageProvider: {
            provider: string;

            micro_usd: number;

            tokens: number;

            records: number;
        };
        UsageSeries: {
            days: components["schemas"]["UsageDay"][];
            providers: components["schemas"]["UsageProvider"][];
        };
        OverviewDay: {
            day: string;
            findings: number;
            runs: number;
            merges: number;
            deployments: number;
        };
        OverviewSeverity: {
            severity: string;
            count: number;
        };
        ConnectionUpdate: {
            name: string;
            namespace: string;
        };
        SetupStep: {

            id: "forge" | "repository" | "scan" | "model" | "pricing" | "budget" | "runner" | "assignment" | "policy" | "publish" | "server";
            done: boolean;
            reason?: string;
            repository_id?: string;
            connection_id?: string;
        };
        Setup: {
            steps: components["schemas"]["SetupStep"][];
        };
        Autopilot: {
            enabled: boolean;
            status: string;

            checked_at?: string;
            active: number;
            queued: number;
            skipped: number;

            version: number;
        };
        AutopilotUpdate: {
            enabled: boolean;
        };
    };
    responses: never;
    parameters: never;
    requestBodies: never;
    headers: never;
    pathItems: never;
}
export type $defs = Record<string, never>;
export interface operations {
    health: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Health"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    readiness: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Health"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    metadata: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Meta"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    session: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Session"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    revokeSessions: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    repositories: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
                q?: string;
                provider?: string;
                team_id?: string;
                status?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepositoryPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    login: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            302: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    oidcCallback: {
        parameters: {
            query: {
                state: string;
                code: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            302: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    logout: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    bootstrap: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["BootstrapRequest"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Organisation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listMemberships: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MembershipPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putMembership: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;

                "If-Match": string;
            };
            path: {
                orgID: string;
                userID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MembershipInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Membership"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    deleteMembership: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;

                "If-Match": string;
            };
            path: {
                orgID: string;
                userID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listTeams: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TeamPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putTeam: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;

                "If-Match": string;
            };
            path: {
                orgID: string;
                teamID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TeamInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Team"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    deleteTeam: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;

                "If-Match": string;
            };
            path: {
                orgID: string;
                teamID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listConnections: {
        parameters: {
            query?: {
                kind?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConnectionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createConnection: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ConnectionCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getConnection: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    revokeConnection: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    updateConnection: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ConnectionUpdate"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    deleteConnection: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    testConnection: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    rotateCredential: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CredentialRotation"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    setPrivateRoute: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PrivateRouteChange"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listForgesConnections: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConnectionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createForgesConnection: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ConnectionCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listModelsConnections: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConnectionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createModelsConnection: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ConnectionCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listModelCatalog: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ModelCatalogRequest"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ModelCatalog"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listAgentsConnections: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConnectionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createAgentsConnection: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ConnectionCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listDeliveryConnections: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ConnectionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createDeliveryConnection: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ConnectionCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    rewrapCredential: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Connection"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getEffectivePolicy: {
        parameters: {
            query?: {
                repository_id?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResolvedPolicy"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getRepositoryPolicy: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repoID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ResolvedPolicy"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listPolicyVersions: {
        parameters: {
            query?: {
                scope_kind?: string;
                scope_id?: string;
                cursor?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PolicyVersionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createPolicyVersion: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PolicyVersionCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PolicyVersion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getPolicyVersion: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                versionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PolicyVersion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    simulatePolicy: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                versionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PolicySimulateRequest"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PolicySimulation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    activatePolicy: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                versionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PolicyActivateRequest"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PolicyBindingVersion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listTasks: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
                state?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["TaskPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    enqueueTask: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["TaskCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Task"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getTask: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                taskID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Task"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelTask: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                taskID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Task"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    resumeTask: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                taskID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Task"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    setPause: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                scopeKind: string;
                scopeID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PauseInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Pause"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    replayEvents: {
        parameters: {
            query?: {
                after?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EventPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    streamEvents: {
        parameters: {
            query?: {
                after?: number;
            };
            header?: {
                "Last-Event-ID"?: string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "text/event-stream": string;
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getBudget: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                scopeKind: string;
                scopeID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BudgetLimit"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putBudget: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                scopeKind: string;
                scopeID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["BudgetLimitInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BudgetLimit"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getBudgetRoute: {
        parameters: {
            query?: {
                model?: string;
                route?: string;
            };
            header?: never;
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BudgetRoute"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putBudgetRoute: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["BudgetRouteInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BudgetRoute"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getReservation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                reservationID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["BudgetReservation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listRunnerPools: {
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
                q?: string;
                state?: "active" | "draining" | "revoked";
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerPoolPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createRunnerPool: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RunnerPoolInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerPool"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getRunnerPool: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                poolID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerPool"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    updateRunnerPool: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                poolID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RunnerPoolInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerPool"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createRunnerEnrollment: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                poolID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["EnrollmentToken"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listRunners: {
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                orgID: string;
                poolID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    revokeRunner: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                runnerID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    artifactMetadata: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                artifactID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ArtifactMetadata"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    downloadArtifact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                artifactID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/octet-stream": string;
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    enrollRunner: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": {
                    name: string;
                };
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerCredential"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    rotateRunner: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerCredential"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    claimRunnerJob: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerAssignment"];
                };
            };

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerHeartbeat: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RunnerHeartbeat"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerProgress: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": {
                    state: components["schemas"]["TaskState"];
                };
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Task"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerResult: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RunnerCompletion"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Task"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    uploadRunnerArtifact: {
        parameters: {
            query?: never;
            header: {
                "X-Artifact-Name": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "text/plain": string;
                "application/json": string;
                "text/x-diff": string;
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ArtifactMetadata"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    downloadRunnerArtifact: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                artifactID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/octet-stream": string;
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    invokeRunnerOperation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                operation: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": {

                    connection_id: string;
                    input: {
                        [key: string]: unknown;
                    };
                };
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: unknown;
                    };
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    pollPrivateGrant: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": Record<string, never>;
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PrivateGrant"];
                };
            };

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    completePrivateGrant: {
        parameters: {
            query?: never;
            header: {
                "X-Private-Result-Capability": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["PrivateCompletion"];
            };
        };
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listInventoryJobs: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryJobPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    startInventorySync: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["InventorySyncInput"];
            };
        };
        responses: {

            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryJob"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getInventoryJob: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                syncID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryJob"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelInventoryJob: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                syncID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryJob"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listInventoryCandidates: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
                syncID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryCandidatePage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    importInventoryCandidates: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                syncID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["InventoryImportInput"];
            };
        };
        responses: {

            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryJob"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getInventoryRepository: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Repository"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listInventoryChanges: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryChangePage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getInventoryWebhook: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InventoryWebhook"];
                };
            };

            404: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    rotateInventoryWebhook: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["IssuedInventoryWebhook"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    revokeInventoryWebhook: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    receiveForgeWebhook: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                endpointID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": {
                    [key: string]: unknown;
                };
            };
        };
        responses: {

            202: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listFindings: {
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
                q?: string;
                repository_id?: string;
                state?: string;
                category?: string;
                severity?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["FindingPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getFinding: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                findingID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Finding"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    updateFinding: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
            };
            path: {
                orgID: string;
                findingID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["FindingUpdate"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Finding"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    importAdvisory: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AdvisoryInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Finding"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getMaintenanceConfig: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MaintenanceConfig"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putMaintenanceConfig: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
            };
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MaintenanceConfig"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MaintenanceConfig"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getDiscoveryScan: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DiscoveryScan"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    startDiscoveryScan: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            202: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DiscoveryScan"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listRepairRecipes: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        [key: string]: string;
                    };
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    previewRepair: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RepairInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairPreview"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    enqueueRepair: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RepairInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getRepairRun: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                taskID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    reconcileRepair: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                taskID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRepairContext: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairExecution"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRepairRun: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRepairSnapshot: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                sha: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["PinnedSnapshot"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRepairReport: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RepairReport"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRepairStage: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRepairPublish: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RepairPublication"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerModelTurn: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["ModelTurn"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["ModelTurnResult"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getRepairBaseline: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repoID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        run: components["schemas"]["RepairRun"] | null;
                    };
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runnerRecordNativeChecks: {
        parameters: {
            query?: never;
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["RepairPublication"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["RepairRun"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getMergeConfiguration: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeConfiguration"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putMergeConfiguration: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MergeConfiguration"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeConfiguration"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    previewProtectedMerge: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                repositoryID: string;
                changeID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MergePreviewRequest"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeGate"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listMergeOperations: {
        parameters: {
            query: {
                repository_id: string;
                change_id?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeOperationPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    requestProtectedMerge: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MergeRequestInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getMergeOperation: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                operationID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelMergeOperation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                operationID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    reconcileMergeOperation: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                operationID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listBotRevalidations: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                repositoryID: string;
                changeID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        items: components["schemas"]["BotRevalidation"][];
                    };
                };
            };
        };
    };
    listDeploymentConfigurations: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentConfigurationPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putDeploymentConfiguration: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                environment: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DeploymentConfiguration"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentConfiguration"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    previewDeployment: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                environment: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DeploymentPreviewInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentGate"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listDeployments: {
        parameters: {
            query?: {
                repository_id?: string;
                environment?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentOperationPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    requestDeployment: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DeploymentRequestInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getDeployment: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                deploymentID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentDetail"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    observeDeployment: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                deploymentID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    submitDeploymentHealth: {
        parameters: {
            query?: never;
            header: {
                "X-Reforge-Signature": string;
            };
            path: {
                orgID: string;
                deploymentID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DeploymentHealth"];
            };
        };
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelDeployment: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                deploymentID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listDeliveryWorkflows: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                repoID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": {
                        items: components["schemas"]["NativeDeliveryWorkflow"][];
                    };
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    trackDeployment: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                environment: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DeploymentTrackInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["DeploymentOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listGitOpsConfigurations: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsConfigurationPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putGitOpsConfiguration: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                environment: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["GitOpsConfiguration"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsConfiguration"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    previewGitOpsPromotion: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                environment: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["GitOpsPreviewInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsGate"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listGitOpsPromotions: {
        parameters: {
            query?: {
                environment?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsPromotionPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    requestGitOpsPromotion: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["DeploymentRequestInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsPromotion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getGitOpsPromotion: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsDetail"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelGitOpsPromotion: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsPromotion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    observeGitOpsPromotion: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsPromotion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    continueGitOpsPromotion: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitOpsPromotion"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    previewGitOpsMerge: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MergePreviewRequest"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeGate"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    requestGitOpsMerge: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["MergeRequestInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["MergeOperation"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    submitGitOpsHealth: {
        parameters: {
            query?: never;
            header: {
                "X-Reforge-Signature": string;
            };
            path: {
                orgID: string;
                promotionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["GitOpsHealth"];
            };
        };
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listUsage: {
        parameters: {
            query?: {
                repository_id?: string;
                team_id?: string;
                recipe?: string;
                provider?: string;
                connection_id?: string;
                state?: string;
                since?: string;
                until?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UsagePage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getOverview: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Overview"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getSetup: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Setup"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    summarizeUsage: {
        parameters: {
            query?: {
                repository_id?: string;
                team_id?: string;
                recipe?: string;
                provider?: string;
                connection_id?: string;
                state?: string;
                since?: string;
                until?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UsageSummary"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    seriesUsage: {
        parameters: {
            query?: {
                repository_id?: string;
                team_id?: string;
                recipe?: string;
                provider?: string;
                connection_id?: string;
                state?: string;
                since?: string;
                until?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["UsageSeries"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listAuditEvents: {
        parameters: {
            query?: {
                repository_id?: string;
                actor_id?: string;
                action?: string;
                since?: string;
                until?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AuditEventPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    exportAuditPage: {
        parameters: {
            query?: {
                repository_id?: string;
                actor_id?: string;
                action?: string;
                since?: string;
                until?: string;
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    "X-Next-Cursor"?: string;
                    "X-Export-Complete"?: boolean;
                    [name: string]: unknown;
                };
                content: {
                    "application/x-ndjson": string;
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getAgentQualification: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AgentQualificationResponse"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putAgentQualification: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AgentQualificationInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["AgentQualificationResponse"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    deleteAgentQualification: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                connectionID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listCustomProfiles: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CustomProfilePage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createCustomProfile: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CustomProfileInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CustomProfile"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getCustomProfile: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                profileID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CustomProfile"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    approveCustomProfile: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
            };
            path: {
                orgID: string;
                profileID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CustomProfileApproval"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CustomProfile"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    revokeCustomProfile: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
            };
            path: {
                orgID: string;
                profileID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CustomProfile"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    previewCampaign: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CampaignInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CampaignPreview"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listCampaigns: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
                state?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CampaignPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createCampaign: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CampaignCreateInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Campaign"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getCampaign: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
                campaignID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Campaign"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listCampaignMembers: {
        parameters: {
            query?: {
                cursor?: string;
                limit?: number;
            };
            header?: never;
            path: {
                orgID: string;
                campaignID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CampaignMemberPage"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    startCampaign: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                campaignID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CampaignControlInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Campaign"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    pauseCampaign: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                campaignID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CampaignControlInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Campaign"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    resumeCampaign: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                campaignID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CampaignControlInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Campaign"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelCampaign: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
                "If-Match": string;
            };
            path: {
                orgID: string;
                campaignID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["CampaignControlInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Campaign"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getOrgOIDC: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["OrgOIDCSettings"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    putOrgOIDC: {
        parameters: {
            query?: never;
            header: {

                "If-Match": string;

                "X-CSRF-Token": string;

                Origin: string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["OrgOIDCInput"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["OrgOIDCSettings"];
                };
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            428: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    probeOrgOIDC: {
        parameters: {
            query?: never;
            header: {

                "If-Match": string;

                "X-CSRF-Token": string;

                Origin: string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["OrgOIDCSettings"];
                };
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            422: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            428: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    activateOrgOIDC: {
        parameters: {
            query?: never;
            header: {

                "If-Match": string;

                "X-CSRF-Token": string;

                Origin: string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            428: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    disableOrgOIDC: {
        parameters: {
            query?: never;
            header: {

                "If-Match": string;

                "X-CSRF-Token": string;

                Origin: string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["OrgOIDCSettings"];
                };
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            428: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    listOrgOIDCInvitations: {
        parameters: {
            query?: {
                limit?: number;
                cursor?: string;
            };
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["OrgOIDCInvitationPage"];
                };
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    createOrgOIDCInvitation: {
        parameters: {
            query?: never;
            header: {

                "X-CSRF-Token": string;

                Origin: string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["OrgOIDCInvitationInput"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["CreatedOrgOIDCInvitation"];
                };
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            409: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    revokeOrgOIDCInvitation: {
        parameters: {
            query?: never;
            header: {

                "X-CSRF-Token": string;

                Origin: string;
            };
            path: {
                orgID: string;
                invitationID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    redeemOrgOIDCInvitation: {
        parameters: {
            query?: never;
            header: {

                Origin: string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/x-www-form-urlencoded": {
                    token: string;
                };
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["InvitationAuthorization"];
                };
            };

            400: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            401: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            403: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            429: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            500: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            503: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getGitHubAppSetup: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitHubAppStatus"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    startGitHubAppSetup: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["GitHubAppSetupCreate"];
            };
        };
        responses: {

            201: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["GitHubAppSetup"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    cancelGitHubAppSetup: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                setupID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    githubAppHandoff: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                setupID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "text/html": string;
                };
            };

            303: {
                headers: {
                    Location?: string;
                    [name: string]: unknown;
                };
                content?: never;
            };
        };
    };
    githubManifestCallback: {
        parameters: {
            query?: {
                code?: string;
                state?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            303: {
                headers: {
                    Location?: string;
                    [name: string]: unknown;
                };
                content?: never;
            };
        };
    };
    githubInstallCallback: {
        parameters: {
            query?: {
                installation_id?: string;
                setup_action?: string;
                state?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            303: {
                headers: {
                    Location?: string;
                    [name: string]: unknown;
                };
                content?: never;
            };
        };
    };
    githubOAuthCallback: {
        parameters: {
            query?: {
                code?: string;
                state?: string;
                error?: string;
            };
            header?: never;
            path?: never;
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            303: {
                headers: {
                    Location?: string;
                    [name: string]: unknown;
                };
                content?: never;
            };
        };
    };
    githubAppWebhook: {
        parameters: {
            query?: never;
            header: {
                "X-Hub-Signature-256": string;
                "X-GitHub-Event": string;
            };
            path?: never;
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": Record<string, never>;
            };
        };
        responses: {

            202: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    getAutopilot: {
        parameters: {
            query?: never;
            header?: never;
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Autopilot"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    updateAutopilot: {
        parameters: {
            query?: never;
            header: {
                "If-Match": string;
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
            };
            cookie?: never;
        };
        requestBody: {
            content: {
                "application/json": components["schemas"]["AutopilotUpdate"];
            };
        };
        responses: {

            200: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["Autopilot"];
                };
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
    runRepository: {
        parameters: {
            query?: never;
            header: {
                "X-CSRF-Token": string;
            };
            path: {
                orgID: string;
                repositoryID: string;
            };
            cookie?: never;
        };
        requestBody?: never;
        responses: {

            204: {
                headers: {
                    [name: string]: unknown;
                };
                content?: never;
            };

            default: {
                headers: {
                    [name: string]: unknown;
                };
                content: {
                    "application/json": components["schemas"]["APIError"];
                };
            };
        };
    };
}
