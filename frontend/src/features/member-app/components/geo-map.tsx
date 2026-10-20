"use client";

import Feature from "ol/Feature";
import Map from "ol/Map";
import View from "ol/View";
import Attribution from "ol/control/Attribution";
import Circle from "ol/geom/Circle";
import LineString from "ol/geom/LineString";
import Point from "ol/geom/Point";
import TileLayer from "ol/layer/Tile";
import VectorLayer from "ol/layer/Vector";
import "ol/ol.css";
import { fromLonLat } from "ol/proj";
import OSM from "ol/source/OSM";
import VectorSource from "ol/source/Vector";
import CircleStyle from "ol/style/Circle";
import Fill from "ol/style/Fill";
import Stroke from "ol/style/Stroke";
import Style from "ol/style/Style";
import { Crosshair, LoaderCircle } from "lucide-react";
import { useEffect, useRef } from "react";
import { useMyLocation } from "../lib/geolocation";
import { useT } from "../lib/i18n";
import type { TrackPoint } from "../lib/queries-train";

export interface GeoTrack {
  points: TrackPoint[];
  color?: string;
  width?: number;
  opacity?: number;
  /** Draw start/end markers (default true for single-track maps). */
  markers?: boolean;
}

const dotStyle = (radius: number, color: string, ring: number) =>
  new Style({
    image: new CircleStyle({
      radius,
      fill: new Fill({ color }),
      stroke: new Stroke({ color: "#fff", width: ring }),
    }),
  });

/**
 * Real tile map (OpenLayers + OSM). Used on detail/route/heatmap views;
 * feed thumbnails stay on the lightweight SVG renderer.
 */
export function GeoMap({
  tracks,
  height = 220,
  interactive = true,
  className = "",
  showMe = false,
}: {
  tracks: GeoTrack[];
  height?: number;
  interactive?: boolean;
  className?: string;
  /**
   * Offer a "centre on me" control. Off by default: a map of last Tuesday's
   * run has no business asking where the reader is standing.
   */
  showMe?: boolean;
}) {
  const t = useT();
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);
  const meRef = useRef<VectorSource | null>(null);
  const me = useMyLocation();
  // Track identity churns per fetch; rebuild on the serialized geometry instead.
  const geometryKey = JSON.stringify(
    tracks.map((tr) => [tr.points.length, tr.points[0]?.lat, tr.color]),
  );
  const tracksRef = useRef(tracks);
  useEffect(() => {
    tracksRef.current = tracks;
  });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const current = tracksRef.current;

    const source = new VectorSource();
    for (const track of current) {
      if (track.points.length < 2) continue;
      const coords = track.points.map((p) => fromLonLat([p.lng, p.lat]));
      const line = new Feature(new LineString(coords));
      line.setStyle(
        new Style({
          stroke: new Stroke({
            color: track.color ?? `rgba(61, 43, 32, ${track.opacity ?? 1})`,
            width: track.width ?? 4,
            lineCap: "round",
            lineJoin: "round",
          }),
        }),
      );
      source.addFeature(line);
      if (track.markers ?? current.length === 1) {
        const start = new Feature(new Point(coords[0]!));
        start.setStyle(dotStyle(6, "#f4529a", 2));
        const end = new Feature(new Point(coords[coords.length - 1]!));
        end.setStyle(dotStyle(6, "#2a1024", 2));
        source.addFeatures([start, end]);
      }
    }

    // A separate source for the device's own position, so redrawing the
    // tracks never wipes the dot and vice versa.
    const mine = new VectorSource();
    meRef.current = mine;

    const map = new Map({
      target: container,
      layers: [
        new TileLayer({ source: new OSM() }),
        new VectorLayer({ source }),
        new VectorLayer({ source: mine }),
      ],
      // Tanpa tombol zoom/rotate, tetapi atribusi OSM wajib tampil (lisensi ODbL / kebijakan tile OSM).
      // Ditaruh di kiri atas agar tidak tertutup tombol lokasi dan pesan di bagian bawah peta.
      controls: [new Attribution({ collapsible: false, className: "ol-attribution !top-2 !left-2 !right-auto !bottom-auto" })],
      interactions: interactive ? undefined : [],
      view: new View({ center: fromLonLat([106.82, -6.2]), zoom: 12 }),
    });
    const extent = source.getExtent();
    if (extent && Number.isFinite(extent[0])) {
      map.getView().fit(extent, { padding: [28, 28, 28, 28], maxZoom: 16 });
    }
    mapRef.current = map;
    return () => {
      map.setTarget(undefined);
      mapRef.current = null;
      meRef.current = null;
    };
  }, [geometryKey, interactive]);

  // The blue dot, and the circle the browser is prepared to stand behind.
  useEffect(() => {
    const source = meRef.current;
    const map = mapRef.current;
    if (!source || !map || !me.fix) return;

    source.clear();
    const centre = fromLonLat([me.fix.lng, me.fix.lat]);
    const halo = new Feature(new Circle(centre, me.fix.accuracyM));
    halo.setStyle(
      new Style({
        fill: new Fill({ color: "rgba(37, 99, 235, 0.12)" }),
        stroke: new Stroke({ color: "rgba(37, 99, 235, 0.35)", width: 1 }),
      }),
    );
    const dot = new Feature(new Point(centre));
    dot.setStyle(dotStyle(7, "#2563eb", 2.5));
    source.addFeatures([halo, dot]);
    map
      .getView()
      .animate({
        center: centre,
        zoom: Math.max(map.getView().getZoom() ?? 14, 15),
        duration: 400,
      });
  }, [me.fix]);

  return (
    <div
      className={`relative w-full overflow-hidden rounded-xl border border-nh-line bg-nh-raised ${className}`}
      style={{ height }}
    >
      <div ref={containerRef} className="h-full w-full" />
      {showMe ? (
        <button
          type="button"
          onClick={() => void me.request()}
          disabled={me.locating}
          title={me.error ? t(me.error) : t("Centre on my location")}
          aria-label={t("Centre on my location")}
          className="absolute right-3 bottom-3 flex h-10 w-10 items-center justify-center rounded-full border border-nh-line bg-nh-cream text-nh-ink shadow-md transition active:scale-95 disabled:opacity-60"
        >
          {me.locating ? (
            <LoaderCircle size={18} className="animate-spin" />
          ) : (
            <Crosshair size={18} className={me.fix ? "text-nh-forest" : ""} />
          )}
        </button>
      ) : null}
      {/* A refusal has to say something, or the button just looks broken. */}
      {showMe && me.error ? (
        <p className="absolute right-16 bottom-3 left-3 rounded-lg bg-nh-ink/85 px-2.5 py-1.5 text-[11px] leading-tight text-white">
          {t(me.error)}
        </p>
      ) : null}
    </div>
  );
}
