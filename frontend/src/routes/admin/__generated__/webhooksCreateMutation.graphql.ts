/**
 * @generated SignedSource<<9ad47ff38a027c848552b4c8aa7732a3>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type WebhookFormat = "DISCORD" | "JSON" | "SLACK" | "%future added value";
export type CreateWebhookInput = {
  enabled?: boolean;
  format: WebhookFormat;
  kinds: ReadonlyArray<NotificationKind>;
  name: string;
  url: string;
};
export type webhooksCreateMutation$variables = {
  input: CreateWebhookInput;
};
export type webhooksCreateMutation$data = {
  readonly createWebhook: {
    readonly secret: string;
    readonly webhook: {
      readonly id: string;
      readonly name: string;
    };
  };
};
export type webhooksCreateMutation = {
  response: webhooksCreateMutation$data;
  variables: webhooksCreateMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
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
        "name": "input",
        "variableName": "input"
      }
    ],
    "concreteType": "CreateWebhookPayload",
    "kind": "LinkedField",
    "name": "createWebhook",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "concreteType": "Webhook",
        "kind": "LinkedField",
        "name": "webhook",
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
          }
        ],
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "secret",
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
    "name": "webhooksCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "webhooksCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "231a4d1416bf3b6cdd655e7f3c8abffe",
    "id": null,
    "metadata": {},
    "name": "webhooksCreateMutation",
    "operationKind": "mutation",
    "text": "mutation webhooksCreateMutation(\n  $input: CreateWebhookInput!\n) {\n  createWebhook(input: $input) {\n    webhook {\n      id\n      name\n    }\n    secret\n  }\n}\n"
  }
};
})();

(node as any).hash = "6140ee19590bcb487f3625f8fb94a516";

export default node;
