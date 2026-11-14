"use client";

import { useEffect, useRef, useState } from "react";

import { beachInk } from "@/lib/dither";

const source = "/new/night.jpg";
const cssCell = 3;
const driftMs = 900;

type Plate = {
  cols: number;
  rows: number;
  cell: number;
  pixels: Uint8ClampedArray;
};

export function BeachDither() {
  const rootRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const plateRef = useRef<Plate | null>(null);
  const inkRef = useRef<HTMLCanvasElement | null>(null);
  const phaseRef = useRef(0);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    const root = rootRef.current;
    const canvas = canvasRef.current;
    if (!root || !canvas) return;

    const context = canvas.getContext("2d", { alpha: false });
    if (!context) return;

    const image = new Image();
    image.src = source;

    let cancelled = false;
    let frame = 0;
    let resizeTimer = 0;
    let driftTimer = 0;
    let showed = false;

    const draw = (phase: number) => {
      const plate = plateRef.current;
      if (cancelled || !plate) return;

      let ink = inkRef.current;
      if (!ink) {
        ink = document.createElement("canvas");
        inkRef.current = ink;
      }
      if (ink.width !== plate.cols || ink.height !== plate.rows) {
        ink.width = plate.cols;
        ink.height = plate.rows;
      }
      const inkContext = ink.getContext("2d", { alpha: false });
      if (!inkContext) return;

      const dithered = inkContext.createImageData(plate.cols, plate.rows);
      const { cols, rows, pixels } = plate;
      for (let y = 0; y < rows; y++) {
        const row = y * cols;
        for (let x = 0; x < cols; x++) {
          const index = (row + x) * 4;
          const [red, green, blue] = beachInk(
            pixels[index] ?? 0,
            pixels[index + 1] ?? 0,
            pixels[index + 2] ?? 0,
            x,
            y,
            phase,
          );
          dithered.data[index] = red;
          dithered.data[index + 1] = green;
          dithered.data[index + 2] = blue;
          dithered.data[index + 3] = 255;
        }
      }
      if (cancelled) return;

      inkContext.putImageData(dithered, 0, 0);
      const width = cols * plate.cell;
      const height = rows * plate.cell;
      if (canvas.width !== width || canvas.height !== height) {
        canvas.width = width;
        canvas.height = height;
      }
      context.imageSmoothingEnabled = false;
      context.drawImage(ink, 0, 0, width, height);
      if (!showed) {
        showed = true;
        setReady(true);
      }
    };

    // Safari on iPad grows the visual viewport as the toolbar hides. A cover
    // resample of that change reads as a zoom, and the dither blocks the scroll.
    const toolbarSlack = 180;

    const capture = () => {
      if (cancelled || !image.complete || image.naturalWidth === 0) return;

      const rect = root.getBoundingClientRect();
      const cssWidth = Math.round(rect.width);
      const cssHeight = Math.round(rect.height);
      if (cssWidth < 2 || cssHeight < 2) return;

      const cols = Math.max(1, Math.ceil(cssWidth / cssCell));
      const rows = Math.max(1, Math.ceil(cssHeight / cssCell));
      const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
      const cell = Math.max(1, Math.round(cssCell * pixelRatio));
      const current = plateRef.current;
      if (current && current.cols === cols && current.cell === cell) {
        const heightChange = Math.abs(current.rows - rows) * cssCell;
        if (current.rows === rows || heightChange <= toolbarSlack) return;
      }

      const sample = document.createElement("canvas");
      sample.width = cols;
      sample.height = rows;
      const sampleContext = sample.getContext("2d", {
        alpha: false,
        willReadFrequently: true,
      });
      if (!sampleContext) return;

      const scale = Math.max(
        cols / image.naturalWidth,
        rows / image.naturalHeight,
      );
      const drawnWidth = image.naturalWidth * scale;
      const drawnHeight = image.naturalHeight * scale;
      sampleContext.imageSmoothingEnabled = true;
      sampleContext.drawImage(
        image,
        0,
        (rows - drawnHeight) / 2,
        drawnWidth,
        drawnHeight,
      );

      const pixels = sampleContext.getImageData(0, 0, cols, rows).data;
      plateRef.current = { cols, rows, cell, pixels };
      draw(phaseRef.current);
    };

    let scrolling = false;
    let scrollTimer = 0;
    let visible = true;

    const scheduleCapture = () => {
      if (scrolling) return;
      window.clearTimeout(resizeTimer);
      resizeTimer = window.setTimeout(() => {
        if (cancelled || scrolling) return;
        cancelAnimationFrame(frame);
        frame = window.requestAnimationFrame(capture);
      }, 80);
    };

    const motion = window.matchMedia("(prefers-reduced-motion: reduce)");

    const stopDrift = () => {
      window.clearInterval(driftTimer);
      driftTimer = 0;
    };

    const startDrift = () => {
      stopDrift();
      if (motion.matches || !visible || scrolling) return;
      driftTimer = window.setInterval(() => {
        if (!visible || scrolling) return;
        phaseRef.current = (phaseRef.current + 1) % 64;
        cancelAnimationFrame(frame);
        frame = window.requestAnimationFrame(() => draw(phaseRef.current));
      }, driftMs);
    };

    const onScroll = () => {
      if (!scrolling) {
        scrolling = true;
        stopDrift();
        window.clearTimeout(resizeTimer);
        cancelAnimationFrame(frame);
      }
      window.clearTimeout(scrollTimer);
      scrollTimer = window.setTimeout(() => {
        scrolling = false;
        if (visible) startDrift();
      }, 180);
    };

    const onMotion = () => {
      if (motion.matches) {
        phaseRef.current = 0;
        draw(0);
        stopDrift();
        return;
      }
      startDrift();
    };

    image.decode().then(scheduleCapture).catch(scheduleCapture);
    startDrift();

    const observer = new ResizeObserver(scheduleCapture);
    observer.observe(root);
    const intersection = new IntersectionObserver(([entry]) => {
      visible = entry?.isIntersecting ?? false;
      if (visible && !scrolling) startDrift();
      else stopDrift();
    });
    intersection.observe(root);
    window.addEventListener("resize", scheduleCapture);
    window.addEventListener("scroll", onScroll, { passive: true });
    motion.addEventListener("change", onMotion);

    return () => {
      cancelled = true;
      window.clearTimeout(resizeTimer);
      window.clearTimeout(scrollTimer);
      stopDrift();
      cancelAnimationFrame(frame);
      observer.disconnect();
      intersection.disconnect();
      window.removeEventListener("resize", scheduleCapture);
      window.removeEventListener("scroll", onScroll);
      motion.removeEventListener("change", onMotion);
    };
  }, []);

  return (
    <div ref={rootRef} className="absolute inset-0" aria-hidden>
      <canvas
        ref={canvasRef}
        className={`absolute inset-0 h-full w-full [image-rendering:pixelated] transition-opacity duration-300 ease-out motion-reduce:transition-none ${
          ready ? "opacity-100" : "opacity-0"
        }`}
      />
    </div>
  );
}
