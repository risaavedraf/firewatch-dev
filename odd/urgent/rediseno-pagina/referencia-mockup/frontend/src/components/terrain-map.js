import { sectorRisk } from '../core/selectors.js';
import { chip, escape } from './ui.js';

const shapes = [
  'M180 46 342 68 395 156 328 256 199 236 119 143Z',
  'M395 156 538 96 676 161 631 284 466 318 328 256Z',
  'M199 236 328 256 466 318 411 450 236 432 131 344Z',
  'M631 284 696 365 639 460 500 482 411 450 466 318Z'
];
const centers = [[257, 147], [513, 216], [291, 348], [553, 392]];

export function terrainMap(state) {
  return `<div class="terrain-wrap"><svg class="terrain" viewBox="0 0 800 525" role="img" aria-labelledby="map-title map-description">
    <title id="map-title">Vista territorial por sector</title><desc id="map-description">Vista territorial de referencia. Seleccione un sector con los botones bajo el mapa. Los colores señalan prioridades de atención preventiva.</desc>
    <defs><pattern id="terrain-grid" width="42" height="42" patternUnits="userSpaceOnUse"><path d="M42 0H0V42" fill="none" stroke="#173c3b" stroke-opacity=".05"/></pattern><pattern id="forest" width="18" height="18" patternUnits="userSpaceOnUse"><path d="m4 9 3-5 3 5Z" fill="#54836c" opacity=".14"/></pattern></defs>
    <rect width="800" height="525" fill="#e9eee4"/><rect width="800" height="525" fill="url(#terrain-grid)"/>
    <path d="M0 0h102c48 48 38 110 8 166S100 295 67 342 32 427 39 525H0Z" fill="#c7dadd"/>
    <path d="M100 0c43 63 39 114 6 174s7 86-39 176S37 458 39 525" fill="none" stroke="#9ebec2" stroke-width="3"/>
    <g fill="none" stroke="#bac6b4" stroke-width="1" opacity=".65">
      <path d="M125 20c120 84 109-43 244 7s163 96 298 40 161 47 141 91M120 47c130 83 119-36 247 8s170 104 298 41 140 54 130 98M112 80c126 79 137-25 269 17s154 98 286 39 125 64 130 107M100 114c121 84 156-16 280 18s154 117 287 43 127 55 130 94M103 151c127 74 147-6 281 16s154 118 285 50 119 74 127 83M95 194c127 62 142 2 287 12s159 108 287 49 105 59 130 107M93 234c103 57 142 18 287 8s159 96 287 54 101 83 132 112M80 276c120 65 141 27 291 14s163 77 296 54 109 76 132 105M72 321c124 67 149 12 297 17s163 45 298 55 108 64 132 97M67 370c123 48 141 25 297 17s165 32 303 56 110 55 132 74M62 416c130 42 139 26 302 17s169 29 306 59M54 461c130 35 152 25 310 17s176 32 283 41"/>
    </g>
    <path d="M458 0c-57 125 112 177 63 266s54 170 66 259" fill="none" stroke="#b8d5d7" stroke-width="9"/>
    <path d="M458 0c-57 125 112 177 63 266s54 170 66 259" fill="none" stroke="#8ab4b7" stroke-width="2"/>
    <path d="M705 20 770 215 685 279 714 476 625 525 395 484 240 464 175 312 115 300 144 75Z" fill="url(#forest)"/>
    <g>${state.sectors.map((sector, index) => `<path d="${shapes[index]}" class="map-sector risk-${escape(sectorRisk(state, sector))} ${state.sectorId === sector.id ? 'selected' : ''}"/>`).join('')}</g>
    <path d="M107 525 156 381 238 291 351 255 388 195 453 171 533 96 558 0" fill="none" stroke="#f9f7ed" stroke-width="9"/>
    <path d="M107 525 156 381 238 291 351 255 388 195 453 171 533 96 558 0" fill="none" stroke="#a3997e" stroke-width="2" stroke-dasharray="5 5"/>
    <g fill="#fffefa" stroke="#aeb9a8"><rect x="148" y="344" width="8" height="9"/><rect x="158" y="330" width="12" height="8"/><rect x="170" y="341" width="8" height="12"/><rect x="370" y="198" width="10" height="9"/><rect x="385" y="210" width="8" height="10"/><rect x="397" y="196" width="12" height="8"/></g>
    <g class="map-label">${state.sectors.map((sector, index) => `<g transform="translate(${centers[index][0]} ${centers[index][1]})"><circle r="15" fill="${state.sectorId === sector.id ? '#173c3b' : '#fffefa'}"/><text text-anchor="middle" y="5" fill="${state.sectorId === sector.id ? '#fffefa' : '#173c3b'}">0${index + 1}</text><text class="sector-name" text-anchor="middle" y="34">${escape(sector.name)}</text></g>`).join('')}</g>
    <g transform="translate(748 45)" fill="#385752"><path d="m0-15 6 22-6-4-6 4Z"/><text x="0" y="-21" text-anchor="middle" font-size="11">N*</text></g>
    <text transform="translate(38 226) rotate(-90)" class="water-label">BORDE COSTERO</text>
     <rect x="21" y="477" width="238" height="28" rx="5" fill="#fffefa" opacity=".94"/><text x="34" y="495" font-size="11" fill="#385752">REFERENCIA TERRITORIAL</text>
   </svg><div class="map-caption"><span>Prioridad de atención por sector</span><span>* Orientación de referencia</span></div></div>
   <div class="map-legend" aria-label="Prioridad de atención">${[chip('high'), chip('medium'), chip('low'), chip('not_evaluated')].join('')}</div>
  <div class="sector-picker" role="group" aria-label="Seleccionar sector del esquema">${state.sectors.map((sector, index) => `<button type="button" data-command="SELECT_SECTOR" data-id="${escape(sector.id)}" aria-pressed="${state.sectorId === sector.id}"><span>0${index + 1}</span>${escape(sector.name)}</button>`).join('')}</div>`;
}
