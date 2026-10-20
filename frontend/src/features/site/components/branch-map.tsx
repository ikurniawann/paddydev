"use client";

import Feature from "ol/Feature";
import Map from "ol/Map";
import View from "ol/View";
import Attribution from "ol/control/Attribution";
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
import Text from "ol/style/Text";
import { useEffect, useRef } from "react";
import type { LatLng } from "../lib/distance";

export interface MapPin extends LatLng {
  slug: string;
  label: string;
}

const ME = new Style({
  image: new CircleStyle({ radius: 7, fill: new Fill({ color: "#2563eb" }), stroke: new Stroke({ color: "#fff", width: 2.5 }) }),
});

function pinStyle(label: string, selected: boolean): Style {
  return new Style({
    image: new CircleStyle({
      radius: selected ? 11 : 9,
      fill: new Fill({ color: selected ? "#b8045a" : "#e3066f" }),
      stroke: new Stroke({ color: selected ? "#e3066f" : "#b8045a", width: 3 }),
    }),
    text: new Text({
      text: label,
      offsetY: -18,
      font: "700 12px Nunito, system-ui, sans-serif",
      fill: new Fill({ color: "#2a1024" }),
      stroke: new Stroke({ color: "#fffdf9", width: 3 }),
    }),
  });
}

/** OSM tiles with one pin per branch; clicking a pin reports its slug. */
export function BranchMap({
  pins,
  me,
  selected,
  onSelect,
  height = 360,
  className = "",
}: {
  pins: MapPin[];
  me: LatLng | null;
  selected: string | null;
  onSelect?: (slug: string) => void;
  height?: number;
  className?: string;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);
  const pinsRef = useRef<VectorSource | null>(null);
  const meRef = useRef<VectorSource | null>(null);
  const onSelectRef = useRef(onSelect);
  useEffect(() => {
    onSelectRef.current = onSelect;
  });

  const pinsKey = JSON.stringify(pins.map((p) => [p.slug, p.lat, p.lng]));

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const pinSource = new VectorSource();
    const meSource = new VectorSource();
    pinsRef.current = pinSource;
    meRef.current = meSource;
    for (const pin of pins) {
      const f = new Feature(new Point(fromLonLat([pin.lng, pin.lat])));
      f.setId(pin.slug);
      f.setStyle(pinStyle(pin.label, pin.slug === selected));
      pinSource.addFeature(f);
    }
    const map = new Map({
      target: container,
      layers: [new TileLayer({ source: new OSM() }), new VectorLayer({ source: pinSource }), new VectorLayer({ source: meSource })],
      // Tanpa tombol zoom/rotate, tetapi atribusi OSM wajib tampil (lisensi ODbL / kebijakan tile OSM).
      controls: [new Attribution({ collapsible: false })],
      view: new View({ center: fromLonLat([107.6, -6.9]), zoom: 11 }),
    });
    const extent = pinSource.getExtent();
    if (extent && pins.length > 0 && Number.isFinite(extent[0])) {
      map.getView().fit(extent, { padding: [48, 48, 48, 48], maxZoom: 14 });
    }
    map.on("click", (evt) => {
      map.forEachFeatureAtPixel(evt.pixel, (feature) => {
        const id = feature.getId();
        if (typeof id === "string") onSelectRef.current?.(id);
        return true;
      });
    });
    map.on("pointermove", (evt) => {
      container.style.cursor = map.hasFeatureAtPixel(evt.pixel) ? "pointer" : "";
    });
    mapRef.current = map;
    return () => {
      map.setTarget(undefined);
      mapRef.current = null;
      pinsRef.current = null;
      meRef.current = null;
    };
    // pins are keyed on their serialized positions; `selected` is applied below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pinsKey]);

  useEffect(() => {
    const source = pinsRef.current;
    if (!source) return;
    for (const pin of pins) {
      source.getFeatureById(pin.slug)?.setStyle(pinStyle(pin.label, pin.slug === selected));
    }
    const map = mapRef.current;
    const target = pins.find((p) => p.slug === selected);
    if (map && target) {
      map.getView().animate({ center: fromLonLat([target.lng, target.lat]), duration: 350 });
    }
  }, [pins, selected]);

  useEffect(() => {
    const source = meRef.current;
    const map = mapRef.current;
    if (!source || !map) return;
    source.clear();
    if (!me) return;
    const dot = new Feature(new Point(fromLonLat([me.lng, me.lat])));
    dot.setStyle(ME);
    source.addFeature(dot);
    const all = new VectorSource({ features: [...(pinsRef.current?.getFeatures() ?? []), dot] }).getExtent();
    if (all) map.getView().fit(all, { padding: [48, 48, 48, 48], maxZoom: 13, duration: 400 });
  }, [me]);

  return (
    <div className={`overflow-hidden rounded-card bg-surface-2 shadow-card ${className}`} style={{ height }}>
      <div ref={containerRef} className="h-full w-full" role="img" aria-label="Branch map" />
    </div>
  );
}
