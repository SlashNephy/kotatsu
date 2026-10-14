/**
 * @generated SignedSource<<5e8cb422a1c06fd24a84fc44b380b72e>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type NotificationKind = "RECORDING_COMPLETED" | "RECORDING_FAILED" | "RECORDING_STARTED" | "RESERVATION_CONFLICT" | "STORAGE_LOW" | "TUNER_SERVER_UNREACHABLE" | "%future added value";
export type NotificationPreferenceInput = {
  inApp: boolean;
  kind: NotificationKind;
  ruleReservations?: boolean | null | undefined;
  webPush: boolean;
};
export type notificationsPreferenceMutation$variables = {
  input: NotificationPreferenceInput;
};
export type notificationsPreferenceMutation$data = {
  readonly updateNotificationPreference: {
    readonly inApp: boolean;
    readonly kind: NotificationKind;
    readonly ruleReservations: boolean;
    readonly webPush: boolean;
  };
};
export type notificationsPreferenceMutation = {
  response: notificationsPreferenceMutation$data;
  variables: notificationsPreferenceMutation$variables;
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
    "concreteType": "NotificationPreference",
    "kind": "LinkedField",
    "name": "updateNotificationPreference",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "kind",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "inApp",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "webPush",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "ruleReservations",
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
    "name": "notificationsPreferenceMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "notificationsPreferenceMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "27a42b324362359ab1e335397d1b61e9",
    "id": null,
    "metadata": {},
    "name": "notificationsPreferenceMutation",
    "operationKind": "mutation",
    "text": "mutation notificationsPreferenceMutation(\n  $input: NotificationPreferenceInput!\n) {\n  updateNotificationPreference(input: $input) {\n    kind\n    inApp\n    webPush\n    ruleReservations\n  }\n}\n"
  }
};
})();

(node as any).hash = "474163c9c9d77c5b90d3cd9831aecd39";

export default node;
