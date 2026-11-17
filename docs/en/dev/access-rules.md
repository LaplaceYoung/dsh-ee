# Permissions and access rules: a quick reference

## The three layers

Access control in dsh-ee has three layers. All of them are changed in the admin console and take effect immediately after saving.

| Layer | Question it answers | Where to configure | Who is affected after configuring |
| --- | --- | --- | --- |
| Permission code | Can this person use this feature | "Roles & permissions" | Page visibility, API calls |
| Fields & access rules | Can this field and this record be viewed or changed | "Fields & access rules" | Custom fields on documents, knowledge bases, users, etc. |
| Admin constraints | Does changing critical configuration need approval | "Fields & access rules" | Operations that change field values or configuration |

## How permission codes reach pages and APIs

Menus only decide what is visible; the real boundary is in the API. Hiding a button on a page is not a security boundary.

| Common permission code | What it does | Default for employees |
| --- | --- | --- |
| `portal.view` | Use employee pages such as the Workbench, company apps and developer docs | Yes |
| `registry.publish` | Publish plugins, skills, prompts and apps | Yes |
| `extensions.develop` | Get the extension development skill and command reference | No; an administrator must grant it |
| `plugins.manage` | Review, publish and unpublish capabilities | No |
| `connections.manage` | Connect and configure company connections | No |

Permission codes are synced into the database from the built-in directory in code; permission codes added by a new release are granted to the "Employee" role automatically (if the directory marks a default role).

## Order in which access rules are evaluated

1. Super admin: not limited by rules (unless super admin is turned off in the system).
2. Role permissions: first decide whether the feature itself is visible.
3. Field access rules: decide read / write / deny field by field; rules can carry conditions.
4. Admin constraints: whether a write operation must first submit a change, and who approves it.

Once an earlier layer reaches a conclusion, later layers do not loosen it.

## Configuration entry points and examples

| I want to | Go to |
| --- | --- |
| Give a group of employees access to extension development materials | "Roles & permissions" → grant `extensions.develop` |
| Let only this department see a field | "Fields & access rules" → create a read rule and choose department as the condition |
| Forbid exporting the "Confidential" field in documents | Same as above: add a deny rule and check the API response |
| Require approval before changing a critical field | "Fields & access rules" → Admin constraints → bind a change request |

## How to fit in when writing extensions

- Declare the permission codes and credential names you need in the manifest; do not put credentials in code or the manifest.
- Field read/write rules are configured by administrators; extensions only handle the visible fields returned by the API and do not guess permissions themselves.
- Error codes distinguish "no permission", "not found" and "bad parameter"; do not return all of them as 500.

## Full text in the repository

This page is only a quick reference; rule details, extension install approval and the permission code directory live in the repository docs (links point to the docs after merge):

- [Access control design (docs/access-control.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/access-control.md)
- [Enterprise publishing, review and permissions (docs/enterprise-publishing.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/enterprise-publishing.md)
- [Extension protocol v1 overview · credentials and write operations (docs/extension-protocol/README.md, available after merge)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol/README.md)
- [Extension protocol text (docs/extension-protocol.md)](https://github.com/hikariming/dsh-ee/blob/main/docs/extension-protocol.md)
- [Access configuration skill (extensions/skills/dsh-ee-access-config/SKILL.md)](https://github.com/hikariming/dsh-ee/blob/main/extensions/skills/dsh-ee-access-config/SKILL.md)
