import type { Dict } from './dictionaries'

/**
 * A recursive union of every dotted leaf path in the dictionary, e.g.
 * `'pipeline.title'` or `'companies.colName'`. Used to type `t(key)` so a typo
 * or a removed key is a compile error, not a silent runtime miss.
 */
export type TKey = LeafPaths<Dict>

type LeafPaths<T> = {
  [K in keyof T & string]: T[K] extends string
    ? K
    : `${K}.${LeafPaths<T[K]>}`
}[keyof T & string]

/** Simple `{var}` interpolation map passed to `t(key, vars)`. */
export type TVars = Record<string, string | number>
