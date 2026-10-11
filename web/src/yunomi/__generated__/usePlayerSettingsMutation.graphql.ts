/**
 * @generated SignedSource<<1e5ff6a4333a5fe50676865fa8a97734>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type PlayerSettingsInput = {
  captionsEnabled?: boolean | null | undefined;
  jikkyoEnabled?: boolean | null | undefined;
};
export type usePlayerSettingsMutation$variables = {
  input: PlayerSettingsInput;
};
export type usePlayerSettingsMutation$data = {
  readonly updatePlayerSettings: {
    readonly captionsEnabled: boolean;
    readonly jikkyoEnabled: boolean;
  };
};
export type usePlayerSettingsMutation = {
  response: usePlayerSettingsMutation$data;
  variables: usePlayerSettingsMutation$variables;
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
    "concreteType": "PlayerSettings",
    "kind": "LinkedField",
    "name": "updatePlayerSettings",
    "plural": false,
    "selections": [
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "jikkyoEnabled",
        "storageKey": null
      },
      {
        "alias": null,
        "args": null,
        "kind": "ScalarField",
        "name": "captionsEnabled",
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
    "name": "usePlayerSettingsMutation",
    "selections": (v1/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": (v0/*:: as any*/),
    "kind": "Operation",
    "name": "usePlayerSettingsMutation",
    "selections": (v1/*:: as any*/)
  },
  "params": {
    "cacheID": "b5d09e3b50f6d083a635f475c4240424",
    "id": null,
    "metadata": {},
    "name": "usePlayerSettingsMutation",
    "operationKind": "mutation",
    "text": "mutation usePlayerSettingsMutation(\n  $input: PlayerSettingsInput!\n) {\n  updatePlayerSettings(input: $input) {\n    jikkyoEnabled\n    captionsEnabled\n  }\n}\n"
  }
};
})();

(node as any).hash = "e981da65c92209fef7f9a21f4ee4d1cc";

export default node;
