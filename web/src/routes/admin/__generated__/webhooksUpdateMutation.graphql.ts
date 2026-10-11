/**
 * @generated SignedSource<<620dc997d5365fa01d45ded1d6d21878>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "%future added value";
export type WebhookFormat = "DISCORD" | "JSON" | "SLACK" | "%future added value";
export type UpdateWebhookInput = {
  enabled?: boolean | null | undefined;
  format?: WebhookFormat | null | undefined;
  kinds?: ReadonlyArray<NotificationKind> | null | undefined;
  name?: string | null | undefined;
  url?: string | null | undefined;
};
export type webhooksUpdateMutation$variables = {
  id: string;
  input: UpdateWebhookInput;
};
export type webhooksUpdateMutation$data = {
  readonly updateWebhook: {
    readonly enabled: boolean;
    readonly format: WebhookFormat;
    readonly id: string;
    readonly kinds: ReadonlyArray<NotificationKind>;
    readonly name: string;
    readonly url: string;
  };
};
export type webhooksUpdateMutation = {
  response: webhooksUpdateMutation$data;
  variables: webhooksUpdateMutation$variables;
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
    "name": "webhooksUpdateMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "webhooksUpdateMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "e0ebd0663152e0f09b2040af947ca038",
    "id": null,
    "metadata": {},
    "name": "webhooksUpdateMutation",
    "operationKind": "mutation",
    "text": "mutation webhooksUpdateMutation(\n  $id: ID!\n  $input: UpdateWebhookInput!\n) {\n  updateWebhook(id: $id, input: $input) {\n    id\n    name\n    url\n    format\n    kinds\n    enabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "2d2ebbd2482be5ff902b1495bb25a49c";

export default node;
