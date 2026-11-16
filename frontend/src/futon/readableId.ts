// URL に入れる ID を読みやすくする (#1346)。画面と GraphQL では、今までどおり GraphQL の ID (`型名:ID` の base64url) を使い、
// URL にだけ素の ID (サービスは Mirakurun のサービス ID、録画やルールは UUID) を出す。GraphQL の ID の形を知るのは、ここだけにする。

// IdType は、URL で素の ID にする GraphQL の型。
export type IdType = 'Service' | 'Recording' | 'Rule' | 'WatchRule';

// rawPatterns は、型ごとの素の ID の形。これに合うものだけを素の ID とみなし、ほかの値 (前の形の URL、`new` など) は変えずに通す。
const rawPatterns: Record<IdType, RegExp> = {
  Service: /^\d+$/,
  Recording: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
  Rule: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
  WatchRule: /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
};

function encodeBase64Url(text: string): string {
  return btoa(text).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function decodeBase64Url(value: string): string | null {
  if (!/^[A-Za-z0-9_-]+$/.test(value)) {
    return null;
  }
  try {
    return atob(value.replace(/-/g, '+').replace(/_/g, '/'));
  } catch {
    return null;
  }
}

// toGlobalId は、URL の値 value を GraphQL の ID にする。素の ID なら型名を付けて符号化し、そうでなければそのまま返す。
// 前の形の URL (GraphQL の ID のまま) のブックマークや、送った通知のリンクも開けるようにするためである。
export function toGlobalId(type: IdType, value: string): string {
  return rawPatterns[type].test(value) ? encodeBase64Url(`${type}:${value.toLowerCase()}`) : value;
}

// toRawId は、GraphQL の ID id を URL に出す素の ID にする。型が違うか、素の ID の形でなければ、そのまま返す。
export function toRawId(type: IdType, id: string): string {
  const decoded = decodeBase64Url(id);
  const prefix = `${type}:`;
  if (decoded === null || !decoded.startsWith(prefix)) {
    return id;
  }
  const raw = decoded.slice(prefix.length);

  return rawPatterns[type].test(raw) ? raw : id;
}

// readableParams は、ルートの params の parse と stringify を作る。key は、URL の ID の項目の名前。
export function readableParams<K extends string>(key: K, type: IdType) {
  return {
    parse: (params: Record<K, string>) =>
      ({ [key]: toGlobalId(type, params[key]) }) as Record<K, string>,
    stringify: (params: Record<K, string>) =>
      ({ [key]: toRawId(type, params[key]) }) as Record<K, string>,
  };
}
