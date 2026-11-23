/**
 * brand-icons.ts：少量品牌图标的本地 SVG 映射。
 * 只内置真正用到的品牌图标，避免为个别图标引入整包图标依赖；
 * 路径数据取自各品牌官方标识（24x24 viewBox）。
 * 主要函数/类型：
 *   - BrandIcon：品牌图标数据形状（标题 + path，可选品牌色）
 *   - siX：X（原 Twitter）品牌图标
 */
export interface BrandIcon {
  /** 无障碍标题，同时作为 aria-label */
  title: string;
  /** 24x24 viewBox 下的 SVG path */
  path: string;
  /** 品牌主色（hex，可选） */
  hex?: string;
}

export const siX: BrandIcon = {
  title: "X",
  path: "M14.234 10.162 22.977 0h-2.072l-7.591 8.824L7.251 0H.258l9.168 13.343L.258 24H2.33l8.016-9.318L16.749 24h6.993zm-2.837 3.299-.929-1.329L3.076 1.56h3.182l5.965 8.532.929 1.329 7.754 11.09h-3.182z",
  hex: "000000",
};
