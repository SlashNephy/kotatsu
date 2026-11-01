/**
 * @generated SignedSource<<9269b070a711663db10bf637a653af28>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type AnimeRecordSettingsInput = {
  thresholdPercent: number;
  watchedOnLast: boolean;
  watchingOnEpisode: boolean;
  watchingOnFirst: boolean;
};
export type accountSettingsMutation$variables = {
  input: AnimeRecordSettingsInput;
};
export type accountSettingsMutation$data = {
  readonly updateAnimeRecordSettings: {
    readonly thresholdPercent: number;
  };
};
export type accountSettingsMutation = {
  response: accountSettingsMutation$data;
  variables: accountSettingsMutation$variables;
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
    "concreteType": "AnimeRecordSettings",
    "kind": "LinkedField",
    "name": "updateAnimeRecordSettings",
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
    "name": "accountSettingsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "accountSettingsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "c6024ca8eeb64e99bb7182d860cb69bc",
    "id": null,
    "metadata": {},
    "name": "accountSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation accountSettingsMutation(\n  $input: AnimeRecordSettingsInput!\n) {\n  updateAnimeRecordSettings(input: $input) {\n    thresholdPercent\n  }\n}\n"
  }
};
})();

(node as any).hash = "8c748c96b62995c0b1bf44b22c4789cc";

export default node;
