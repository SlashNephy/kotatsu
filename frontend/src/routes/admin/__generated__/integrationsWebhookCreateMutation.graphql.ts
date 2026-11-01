/**
 * @generated SignedSource<<798ff14469cf4d1f06914c414c10aba8>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "ANIME_RECORDED" | "ANIME_RECORD_FAILED" | "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "WATCH_REMINDER" | "%future added value";
export type WebhookFormat = "DISCORD" | "JSON" | "SLACK" | "%future added value";
export type CreateWebhookInput = {
  enabled?: boolean;
  format: WebhookFormat;
  kinds: ReadonlyArray<NotificationKind>;
  name: string;
  url: string;
};
export type integrationsWebhookCreateMutation$variables = {
  input: CreateWebhookInput;
};
export type integrationsWebhookCreateMutation$data = {
  readonly createWebhook: {
    readonly secret: string;
    readonly webhook: {
      readonly id: string;
      readonly name: string;
    };
  };
};
export type integrationsWebhookCreateMutation = {
  response: integrationsWebhookCreateMutation$data;
  variables: integrationsWebhookCreateMutation$variables;
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
    "name": "integrationsWebhookCreateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "integrationsWebhookCreateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b50c94637e85f48eeebc7ee1a91acde1",
    "id": null,
    "metadata": {},
    "name": "integrationsWebhookCreateMutation",
    "operationKind": "mutation",
    "text": "mutation integrationsWebhookCreateMutation(\n  $input: CreateWebhookInput!\n) {\n  createWebhook(input: $input) {\n    webhook {\n      id\n      name\n    }\n    secret\n  }\n}\n"
  }
};
})();

(node as any).hash = "3ff4dc5cb05bf19e3d626b63dcf4cd35";

export default node;
