/**
 * @generated SignedSource<<1b7cb21ecfdb2b606eaed8d178274477>>
 * @lightSyntaxTransform
 */

/* tslint:disable */
/* eslint-disable */
// @ts-nocheck

import { ReaderFragment } from 'relay-runtime';
import { FragmentRefs } from "relay-runtime";
export type ReservationPeers_assignment$data = {
  readonly peers: ReadonlyArray<{
    readonly id: string;
    readonly name: string;
    readonly service: {
      readonly name: string;
    };
  }>;
  readonly " $fragmentType": "ReservationPeers_assignment";
};
export type ReservationPeers_assignment$key = {
  readonly " $data"?: ReservationPeers_assignment$data;
  readonly " $fragmentSpreads": FragmentRefs<"ReservationPeers_assignment">;
};

const node: ReaderFragment = (function(){
var v0 = {
  "alias": null,
  "args": null,
  "kind": "ScalarField",
  "name": "name",
  "storageKey": null
};
return {
  "argumentDefinitions": [],
  "kind": "Fragment",
  "metadata": null,
  "name": "ReservationPeers_assignment",
  "selections": [
    {
      "alias": null,
      "args": null,
      "concreteType": "Reservation",
      "kind": "LinkedField",
      "name": "peers",
      "plural": true,
      "selections": [
        {
          "alias": null,
          "args": null,
          "kind": "ScalarField",
          "name": "id",
          "storageKey": null
        },
        (v0/*:: as any*/),
        {
          "alias": null,
          "args": null,
          "concreteType": "Service",
          "kind": "LinkedField",
          "name": "service",
          "plural": false,
          "selections": [
            (v0/*:: as any*/)
          ],
          "storageKey": null
        }
      ],
      "storageKey": null
    }
  ],
  "type": "ReservationAssignment",
  "abstractKey": null
};
})();

(node as any).hash = "d6b46b46ace66b0c039d18db596d191e";

export default node;
