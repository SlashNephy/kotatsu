/**
 * @generated SignedSource<<fb3f9c7583a8aec24c05b2882cf12ef5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type WebhookFormat = "DISCORD" | "JSON" | "SLACK" | "%future added value";
export type UpdateWebhookInput = {
  enabled?: boolean | null | undefined;
  format?: WebhookFormat | null | undefined;
  kinds?: ReadonlyArray<NotificationKind> | null | undefined;
  name?: string | null | undefined;
  url?: string | null | undefined;
};
export type integrationsWebhookUpdateMutation$variables = {
  id: string;
  input: UpdateWebhookInput;
};
export type integrationsWebhookUpdateMutation$data = {
  readonly updateWebhook: {
    readonly enabled: boolean;
    readonly format: WebhookFormat;
    readonly id: string;
    readonly kinds: ReadonlyArray<NotificationKind>;
    readonly name: string;
    readonly url: string;
  };
};
export type integrationsWebhookUpdateMutation = {
  response: integrationsWebhookUpdateMutation$data;
  variables: integrationsWebhookUpdateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "id"
  },
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "input"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "id",
        "variableName": "id"
      },
      {
        "kind": "Variable",
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "Webhook",
    "kind": "LinkedField",
    "name": "updateWebhook",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "id",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "name",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "url",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "format",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kinds",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "enabled",
        "storageKey": null
      }
    ],
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Fragment",
    "metadata": null,
    "name": "integrationsWebhookUpdateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsWebhookUpdateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "6e33481344e063290744e2da2b2442d4",
    "id": null,
    "metadata": {},
    "name": "integrationsWebhookUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsWebhookUpdateMutation(\n  $id: ID!\n  $input: UpdateWebhookInput!\n) {\n  updateWebhook(id: $id, input: $input) {\n    id\n    name\n    url\n    format\n    kinds\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "df3da6a9218e69209c09ba96c21a480a";

export default node;
