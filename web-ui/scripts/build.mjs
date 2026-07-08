import { cp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const dist = resolve(root, 'dist');

// build 生成 Go 服务可直接托管的静态文件目录。
async function build() {
  await rm(dist, { recursive: true, force: true });
  await mkdir(resolve(dist, 'assets'), { recursive: true });

  const html = await readFile(resolve(root, 'index.html'), 'utf8');
  await writeFile(
    resolve(dist, 'index.html'),
    html
      .replace('href="/src/styles.css" data-build-href="/assets/styles.css"', 'href="/assets/styles.css"')
      .replace('src="/src/app.js" data-build-src="/assets/app.js"', 'src="/assets/app.js"'),
  );

  await cp(resolve(root, 'src', 'app.js'), resolve(dist, 'assets', 'app.js'));
  await cp(resolve(root, 'src', 'styles.css'), resolve(dist, 'assets', 'styles.css'));
}

await build();
