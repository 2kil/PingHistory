// 多目标对比用的配色：色相彼此拉开，且在浅色背景上对比度足够。
const PALETTE = [
  '#2563eb', // 蓝
  '#12b76a', // 绿
  '#f79009', // 琥珀
  '#7a5af8', // 紫
  '#ee46bc', // 品红
  '#06aed4', // 青
  '#ef6820', // 橙
  '#667085', // 石板灰
  '#84cc16', // 黄绿
  '#d92d20', // 红
]

export function colorAt(index) {
  if (index < 0) return PALETTE[0]
  return PALETTE[index % PALETTE.length]
}

// 以目标顺序稳定分配颜色，保证同一个目标在不同视图里颜色一致。
export function buildColorMap(targets) {
  const map = new Map()
  targets.forEach((target, index) => map.set(target.id, colorAt(index)))
  return map
}
