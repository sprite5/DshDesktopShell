'use strict';
// 用 DSH 官方 favicon.svg 生成应用图标：
//   build/appicon.png (512)      — wails3 icons 源
//   build/windows/icon.ico       — Windows exe 资源图标（多尺寸 PNG 打包）
//   assets/logo.png (128)        — 设置页 logo / Wails about box
//   assets/tray-template.png (32)— macOS 菜单栏模板图标（黑色 glyph，系统自适应着色）
const fs = require('fs');
const path = require('path');

const SHARP = 'C:/Users/Administrator/AppData/Local/npm-cache/_npx/1e7f6d9597241db0/node_modules/sharp';
const sharp = require(SHARP);

const ROOT = path.join(__dirname, '..');

function buildICO(images) {
  const count = images.length;
  const header = Buffer.alloc(6);
  header.writeUInt16LE(0, 0); // reserved
  header.writeUInt16LE(1, 2); // type: icon
  header.writeUInt16LE(count, 4);
  const entries = [];
  const chunks = [];
  let offset = 6 + 16 * count;
  for (const img of images) {
    const e = Buffer.alloc(16);
    e.writeUInt8(img.size >= 256 ? 0 : img.size, 0); // width (0 = 256)
    e.writeUInt8(img.size >= 256 ? 0 : img.size, 1); // height
    e.writeUInt8(0, 2); // colors
    e.writeUInt8(0, 3); // reserved
    e.writeUInt16LE(1, 4); // planes
    e.writeUInt16LE(32, 6); // bpp
    e.writeUInt32LE(img.buf.length, 8);
    e.writeUInt32LE(offset, 12);
    offset += img.buf.length;
    entries.push(e);
    chunks.push(img.buf);
  }
  return Buffer.concat([header, ...entries, ...chunks]);
}

(async () => {
  const favicon = fs.readFileSync('C:/Users/Administrator/AppData/Local/npm-cache/_npx/1e7f6d9597241db0/node_modules/@deepseek-ai/dsh-web-frontend/dist/favicon.svg', 'utf8');
  const paths = [];
  for (const m of favicon.matchAll(/<path\b[^>]*d="([^"]*)"[^>]*>/g)) {
    // 归一化：去掉 id/style 等属性，只留 d，统一白色
    paths.push('<path d="' + m[1] + '"/>');
  }
  if (paths.length === 0) throw new Error('favicon.svg: no <path> found');
  const inner = paths.join('\n');

  // 深蓝圆角方形底 + 白色官方 logo（10% 边距）
  const appSvg = '<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">'
    + '<rect width="512" height="512" rx="112" fill="#0E1F38"/>'
    + '<g transform="translate(31 31) scale(9)" fill="#FFFFFF">' + inner + '</g>'
    + '</svg>';

  const base = await sharp(Buffer.from(appSvg), { density: 288 }).resize(512, 512).png();

  // macOS 菜单栏模板图标：纯黑 glyph + 透明底（系统按深浅色自动着色）。
  // 注意模板图不能用彩色，也不带深蓝底。
  const templateSvg = '<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" viewBox="0 0 512 512">'
    + '<g transform="translate(31 31) scale(9)" fill="#000000">' + inner + '</g>'
    + '</svg>';

  await fs.promises.mkdir(path.join(ROOT, 'build/windows'), { recursive: true });
  fs.writeFileSync(path.join(ROOT, 'build/appicon.png'), await base.clone().toBuffer());
  fs.writeFileSync(path.join(ROOT, 'assets/logo.png'), await base.clone().resize(128, 128).png().toBuffer());
  fs.writeFileSync(path.join(ROOT, 'assets/tray.png'), await base.clone().resize(32, 32).png().toBuffer());
  fs.writeFileSync(path.join(ROOT, 'assets/tray-template.png'),
    await sharp(Buffer.from(templateSvg), { density: 288 }).resize(32, 32).png().toBuffer());

  const sizes = [256, 128, 64, 48, 32, 16];
  const bufs = [];
  for (const s of sizes) {
    bufs.push({ size: s, buf: await base.clone().resize(s, s).png().toBuffer() });
  }
  fs.writeFileSync(path.join(ROOT, 'build/windows/icon.ico'), buildICO(bufs));

  const meta = await sharp(Buffer.from(appSvg)).metadata();
  console.log('icon generated:', appSvg.length, 'bytes svg;',
    'ico sizes', sizes.join(','),
    '| png alpha', (await base.clone().toBuffer()).slice(0, 0));
  console.log('OK: build/appicon.png, build/windows/icon.ico, assets/logo.png, assets/tray-template.png');
})().catch((e) => { console.error(e); process.exit(1); });