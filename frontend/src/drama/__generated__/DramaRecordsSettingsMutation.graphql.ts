/**
 * @generated SignedSource<<aed3198c35e2658b42ddcbd43e1fa44b>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type DramaRecordSettingsInput = {
  thresholdPercent: number;
};
export type DramaRecordsSettingsMutation$variables = {
  input: DramaRecordSettingsInput;
};
export type DramaRecordsSettingsMutation$data = {
  readonly updateDramaRecordSettings: {
    readonly thresholdPercent: number;
  };
};
export type DramaRecordsSettingsMutation = {
  response: DramaRecordsSettingsMutation$data;
  variables: DramaRecordsSettingsMutation$variables;
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
    "concreteType": "DramaRecordSettings",
    "kind": "LinkedField",
    "name": "updateDramaRecordSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "thresholdPercent",
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
    "name": "DramaRecordsSettingsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "DramaRecordsSettingsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "98168b86b6cdfb4485443222469a7e2c",
    "id": null,
    "metadata": {},
    "name": "DramaRecordsSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation DramaRecordsSettingsMutation(\n  $input: DramaRecordSettingsInput!\n) {\n  updateDramaRecordSettings(input: $input) {\n    thresholdPercent\n  }\n}\n"
  }
};
})();

(node as any).hash = "ae9f8f5786cd783c196175c037530225";

export default node;
