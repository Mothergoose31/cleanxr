# Multi-scale CLEAN Implementation for Radio Interferometry

Implementation of the Multi-scale CLEAN algorithm for processing radio interferometric data in ACB format. Optimized for high dynamic range imaging of compact sources (AGN jets, blazar cores).

## Features
- Multi-scale deconvolution for resolving complex source structures
- Viridis colormap implementation for scientifically accurate visualization
- 2K resolution support for presentation-quality images
- Computationally efficient Go implementation for rapid data processing

## Installation
```bash
go build -o clean_acb ./cmd/clean_acb
```

## Usage
```bash
./clean_acb -input [acb_file] -output [image.png] [options]
```

| Parameter | Description | Default |
|-----------|-------------|---------|
| `-input`  | Input ACB file (required) | - |
| `-output` | Output filename | cleaned_image.png |
| `-scales` | Number of scales (3-7 recommended) | 5 |
| `-size`   | Image dimension (px) | 256 |
| `-2k`     | 2048×2048 output YOLO | false |

## Example
BL Lacertae (J2202+4216) at 213 GHz:
```bash
./clean_acb -input ./E18A24.0.bin0000.source0000.acb -output cleaned_bllac.png -size 128 -scales 3
```

## Implementation Notes
The algo uses Gaussian basis functions at multiple spatial scales.

## Acknowledgments
Viridis colormap: Stéfan van der Walt, Nathaniel Smith, and Eric Firing (Matplotlib)