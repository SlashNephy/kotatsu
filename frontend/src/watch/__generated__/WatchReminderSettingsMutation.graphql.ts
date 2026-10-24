/**
 * @generated SignedSource<<453373abff19b06920e5b3f65581b12b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type WatchReminderSettingsMutation$variables = {
  minutesBefore: number;
};
export type WatchReminderSettingsMutation$data = {
  readonly updateWatchReminderSettings: {
    readonly minutesBefore: number;
  };
};
export type WatchReminderSettingsMutation = {
  response: WatchReminderSettingsMutation$data;
  variables: WatchReminderSettingsMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "defaultValue": null,
    "kind": "LocalArgument",
    "name": "minutesBefore"
  }
],
v1 = [
  {
    "alias": null,
    "args": [
      {
        "kind": "Variable",
        "name": "minutesBefore",
        "variableName": "minutesBefore"
      }
    ],
    "concreteType": "WatchReminderSettings",
    "kind": "LinkedField",
    "name": "updateWatchReminderSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "minutesBefore",
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
    "name": "WatchReminderSettingsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "WatchReminderSettingsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "9e2aa0b5f10082488c82f7d289533827",
    "id": null,
    "metadata": {},
    "name": "WatchReminderSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation WatchReminderSettingsMutation(\n  $minutesBefore: Int!\n) {\n  updateWatchReminderSettings(minutesBefore: $minutesBefore) {\n    minutesBefore\n  }\n}\n"
  }
};
})();

(node as any).hash = "8e8ffb565e4b80b94ada76aa625cd065";

export default node;
