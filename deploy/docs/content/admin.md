# Teams and identity

The **Organisation** route manages teams, members, organisation identity and invitations. Only owners can open **Members** and **Identity**. Owners can assign `owner`, `admin`, `maintainer`, `reviewer` or `viewer` roles. Keep owner access limited; organisation identity and invitation settings require owner role.

## Teams and membership

Create teams and assign repositories in **Teams**. In **Members**, owners can change a member role and repository scope or remove the member. Role changes use the current version and are audited. Removing the last owner is rejected.

## Organisation sign-in

Use **Organisation → Identity** to configure an organisation’s OIDC provider. This is separate from installation-wide identity: the operator configures installation sign-in and recovery at install time; an owner configures optional organisation sign-in in the browser.

1. Enter the HTTPS issuer URL, client ID and client secret, then select **Save draft**. A blank secret on later saves keeps the stored secret. The UI never reads it back.
2. Select **Probe issuer metadata**. A successful probe verifies discovery metadata only; it does not test client credentials or user login. Failed probes clear verification. Probe cooldowns return a wait period.
3. When status is **Metadata verified**, select **Activate login**. Only the current verified version can be activated. The configured issuer must meet the network and URL restrictions; private network issuers and custom HTTPS ports are not supported.
4. Organisation sign-in uses existing membership for the exact issuer and subject. It does not create or link accounts by email. Keep an owner able to use installation sign-in for recovery.
5. Select **Disable** to stop organisation login. Disabling invalidates existing organisation sessions on their next request. Saving a changed configuration creates a new draft and requires another probe and activation.

The local signed-issuer test fixture has contract coverage. Hosted customer identity providers have not been certified in this release; a successful metadata probe alone is not that certification.

## Invitations

Create invitations while organisation login is active. Owners can still review and revoke unused invitations after disabling login.

1. Under **Identity → Invitations**, enter the recipient’s email, choose a role and expiry (1, 7 or 30 days), then select **Create invitation**.
2. Copy the returned link immediately. It is shown once and contains a one-time token in its URL fragment. The invitation stores only a token hash.
3. The recipient opens the link and selects **Accept invitation**. The token stays in the browser fragment until the page removes it, then is sent in a same-origin form request. The recipient must sign in through the active organisation provider with a verified email matching the invitation.
4. Review invitation status in the list. Owners can revoke unused invitations. Expired, redeemed, revoked or provider-rotated invitations cannot be used. Revocation removes the invitation.

Invitations grant only the selected role in that organisation. Email-domain matching and automatic email-based account linking are not supported.

Repository access can be granted through a team or directly to a member. A member with `all_repositories` bypasses per-repository grants and should be reserved for owners and administrators.
