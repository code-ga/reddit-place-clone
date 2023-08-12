declare module "image-pixels" {
  interface Image {
    data: Uint8Array;
    width: number;
    height: number;
  }
  export default function pixel(src: string): Promise<Image>;
}
