/**
 * @generated SignedSource<<19ccf428a244ed631f01a3508671e6b5>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ConcreteRequest } from 'relay-runtime';
export type CalendarFeedDeleteMutation$variables = Record<PropertyKey, never>;
export type CalendarFeedDeleteMutation$data = {
  readonly deleteCalendarFeed: boolean;
};
export type CalendarFeedDeleteMutation = {
  response: CalendarFeedDeleteMutation$data;
  variables: CalendarFeedDeleteMutation$variables;
};

const node: ConcreteRequest = (function(){
var v0 = [
  {
    "alias": null,
    "args": null,
    "kind": "ScalarField",
    "name": "deleteCalendarFeed",
    "storageKey": null
  }
];
return {
  "fragment": {
    "argumentDefinitions": [],
    "kind": "Fragment",
    "metadata": null,
    "name": "CalendarFeedDeleteMutation",
    "selections": (v0/*:: as any*/),
    "type": "Mutation",
    "abstractKey": null
  },
  "kind": "Request",
  "operation": {
    "argumentDefinitions": [],
    "kind": "Operation",
    "name": "CalendarFeedDeleteMutation",
    "selections": (v0/*:: as any*/)
  },
  "params": {
    "cacheID": "0a38cc25d87c8906a4f610266343760d",
    "id": null,
    "metadata": {},
    "name": "CalendarFeedDeleteMutation",
    "operationKind": "mutation",
    "text": "mutation CalendarFeedDeleteMutation {\n  deleteCalendarFeed\n}\n"
  }
};
})();

(node as any).hash = "449b530e97eb558a66bcdd1e1824f3dc";

export default node;
