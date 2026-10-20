"use client";

import Feature from "ol/Feature";
import Map from "ol/Map";
import View from "ol/View";
import Attribution from "ol/control/Attribution";
import Point from "ol/geom/Point";
import { circular } from "ol/geom/Polygon";
import TileLayer from "ol/layer/Tile";
import VectorLayer from "ol/layer/Vector";
import "ol/ol.css";
import { fromLonLat, toLonLat } from "ol/proj";
import OSM from "ol/source/OSM";
import VectorSource from "ol/source/Vector";
import CircleStyle from "ol/style/Circle";
import Fill from "ol/style/Fill";
import Stroke from "ol/style/Stroke";
import Style from "ol/style/Style";
import { useEffect, useRef } from "react";
import { resolveRadiusM, roundCoord, type LatLngPoint } from "@/lib/maps/coords";

const PIN = new Style({
  image: new CircleStyle({ radius: 8, fill: new Fill({ color: "#dc2626" }), stroke: new Stroke({ color: "#fff", width: 2.5 }) }),
});
const AREA = new Style({
  fill: new Fill({ color: "rgba(37, 99, 235, 0.12)" }),
  stroke: new Stroke({ color: "rgba(37, 99, 235, 0.6)", width: 1.5 }),
});
const BANDUNG: [number, number] = [107.6, -6.9];
const key = (p: LatLngPoint) => `${p.lat},${p.lng}`;

/**
 * Peta OpenLayers + OSM (stack yang sama dengan BranchMap situs dan GeoMap
 * member app) untuk memilih satu titik: klik peta memindahkan pin. radiusM
 * menggambar area absensi (lingkaran geodesik; kosong = radius bawaan). Titik
 * dari luar (input diketik) membuat peta terbang ke titik itu; klik di peta
 * tidak. Kontrol hanya atribusi OSM (wajib oleh lisensi); tanpa tombol
 * zoom/rotasi supaya klik di peta tidak pernah memicu zoom.
 */
export function LocationPickerMap({
  value,
  radiusM,
  onPick,
  height = 280,
}: {
  value: LatLngPoint | null;
  radiusM?: number | null;
  onPick: (point: LatLngPoint) => void;
  height?: number;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<Map | null>(null);
  const markRef = useRef<VectorSource | null>(null);
  const pickedRef = useRef<string | null>(null);
  const onPickRef = useRef(onPick);
  useEffect(() => {
    onPickRef.current = onPick;
  });

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const mark = new VectorSource();
    markRef.current = mark;
    const map = new Map({
      target: container,
      layers: [new TileLayer({ source: new OSM() }), new VectorLayer({ source: mark })],
      controls: [new Attribution({ collapsible: false })],
      view: new View({ center: fromLonLat(BANDUNG), zoom: 11 }),
    });
    map.on("click", (evt) => {
      const [lng, lat] = toLonLat(evt.coordinate);
      const point = { lat: roundCoord(lat), lng: roundCoord(lng) };
      pickedRef.current = key(point);
      onPickRef.current(point);
    });
    mapRef.current = map;
    return () => {
      map.setTarget(undefined);
      map.dispose();
      mapRef.current = null;
      markRef.current = null;
    };
  }, []);

  const lat = value?.lat;
  const lng = value?.lng;
  const radius = resolveRadiusM(radiusM);
  useEffect(() => {
    const mark = markRef.current;
    const map = mapRef.current;
    if (!mark || !map) return;
    mark.clear();
    if (lat === undefined || lng === undefined) return;
    const area = new Feature(circular([lng, lat], radius, 64).transform("EPSG:4326", "EPSG:3857"));
    area.setStyle(AREA);
    mark.addFeature(area);
    const pin = new Feature(new Point(fromLonLat([lng, lat])));
    pin.setStyle(PIN);
    mark.addFeature(pin);
    if (pickedRef.current !== key({ lat, lng })) {
      const view = map.getView();
      view.animate({ center: fromLonLat([lng, lat]), zoom: Math.max(view.getZoom() ?? 11, 17), duration: 300 });
    }
    pickedRef.current = null;
  }, [lat, lng, radius]);

  return (
    <div className="overflow-hidden rounded-lg border border-gray-200" style={{ height }}>
      <div ref={containerRef} className="h-full w-full cursor-crosshair" role="application" aria-label="Peta pilih lokasi toko" />
    </div>
  );
}
