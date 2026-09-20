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
        get?: never;
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
        Meta: {
            name: string;
            version: string;

            edition: "hosted" | "self-hosted";
            development: boolean;
            fixture_auth: boolean;
            bootstrap_required?: boolean;
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
        };
        Runner: {

            id: string;

            org_id: string;

            pool_id: string;
            name: string;
            state: string;

            version: number;

            credential_expires_at: string;
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
}
