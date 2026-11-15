/**
 * 文案表的类型工具。
 *
 * 中文（zh.ts）是源语言，也是键集合的唯一来源；英文（en.ts）用 `satisfies Messages<typeof zh>`
 * 镜像它——zh 里有、en 里漏了的键会直接编译报错。es / pt / hi 是增量表（DeepPartial），
 * 允许只写一部分键，缺的键在运行时回退英文。
 */

/** 把中文文案的结构放宽成「同样的键、值是任意字符串」，英文按它来写。 */
export type Messages<T> = {
  [K in keyof T]: T[K] extends string ? string : T[K] extends object ? Messages<T[K]> : never;
};

/** 增量语言表（es / pt / hi）：结构相同，但每个命名空间、每个键都可以先不写。 */
export type DeepPartial<T> = {
  [K in keyof T]?: T[K] extends string ? string : T[K] extends object ? DeepPartial<T[K]> : never;
};

/** 嵌套文案对象的点分键路径（例如 "statusView.account"），t() 只接受这些键。 */
export type DotPaths<T, Prefix extends string = ""> = {
  [K in keyof T & string]: T[K] extends string
    ? `${Prefix}${K}`
    : T[K] extends object
      ? DotPaths<T[K], `${Prefix}${K}.`>
      : never;
}[keyof T & string];
