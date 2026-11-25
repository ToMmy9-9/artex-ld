/**
 * abyss-backdrop.tsx：Abyss 深空主题的氛围背板。
 *
 * 为什么单独成一个组件，而不是继续放在 `html` 的 background 里：
 *   1. **视差**需要按鼠标位置平移各层，`background-position` 每帧改会触发重绘；
 *      用 `transform: translate3d()` 走合成层，能保持 60fps。
 *   2. **光扫**是一条独立运动的渐变，放在背景图里没法单独动。
 *   3. 各层需要不同的位移系数（远景慢、近景快），一个 background 做不到。
 *
 * 约束（性能与正确性）：
 *   - `pointer-events: none` + `aria-hidden`：纯装饰，不能拦交互。
 *   - 仅在 dark + abyss 预设下渲染内容（由 CSS 控制显隐），切主题即刻消失。
 *   - 监听 mousemove 用 rAF 节流，且只写两个 CSS 变量（--abyss-px / --abyss-py），
 *     不触发 React 重渲染。
 *   - `prefers-reduced-motion` 下停掉光扫与视差（无障碍 + 省电）。
 */
"use client";

import { useEffect, useRef } from "react";

export function AbyssBackdrop() {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    // 尊重系统"减少动态效果"：直接跳过视差与动画，只留静态底纹。
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)");
    if (reduce.matches) {
      el.dataset.abyssStill = "1";
      return;
    }

    let raf = 0;
    let px = 0;
    let py = 0;

    const apply = () => {
      raf = 0;
      el.style.setProperty("--abyss-px", px.toFixed(4));
      el.style.setProperty("--abyss-py", py.toFixed(4));
    };

    const onMove = (e: MouseEvent) => {
      // 归一化到 [-1, 1]，取屏幕中心为原点。位移幅度刻意很小（见 CSS 的倍率），
      // 目的是"看得出背景有纵深"，不是"背景在跟着鼠标跑"。
      px = (e.clientX / window.innerWidth) * 2 - 1;
      py = (e.clientY / window.innerHeight) * 2 - 1;
      if (!raf) raf = requestAnimationFrame(apply);
    };

    const onLeave = () => {
      px = 0;
      py = 0;
      if (!raf) raf = requestAnimationFrame(apply);
    };

    window.addEventListener("mousemove", onMove, { passive: true });
    window.addEventListener("mouseleave", onLeave, { passive: true });
    return () => {
      window.removeEventListener("mousemove", onMove);
      window.removeEventListener("mouseleave", onLeave);
      if (raf) cancelAnimationFrame(raf);
    };
  }, []);

  return (
    <div ref={ref} className="abyss-backdrop" aria-hidden="true">
      {/* 外壳层只有两样：两记极淡中性灰光池 + 28px 点阵。
          点阵被上面的玻璃 blur 平均成磨砂 —— 那是"玻璃感"的物理来源。 */}
      <div className="abyss-backdrop__glow" />
      <div className="abyss-backdrop__layer" />
    </div>
  );
}
