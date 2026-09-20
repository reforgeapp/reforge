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
}
